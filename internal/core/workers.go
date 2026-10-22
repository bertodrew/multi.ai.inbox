package core

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// OutboundJob is a queued message leased to the sender worker.
type OutboundJob struct {
	ID         string
	OrgID      string
	InboxID    string
	Address    string
	Recipients []string
	RawKey     string
	Attempts   int
	DKIMDomain string
	DKIMSel    string
	DKIMKeyPEM string
	CreatedAt  time.Time
}

// sendLease covers one delivery attempt, not the retry window: FinishOutbound
// replaces next_attempt_at with the real retry time, so the lease only matters
// when a worker crashes mid attempt.
const sendLease = 10 * time.Minute

// ClaimOutbound leases up to n due messages. A lease that expires (crashed
// worker) makes the message due again.
func (s *Service) ClaimOutbound(ctx context.Context, n int) ([]OutboundJob, error) {
	var jobs []OutboundJob
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `update messages m set status = 'sending', attempts = attempts + 1,
				next_attempt_at = now() + $2::interval
			from inboxes i, domains d
			where m.id in (select id from messages q where status in ('queued', 'sending') and next_attempt_at <= now()
				-- paused inboxes and organizations keep their queue until resumed
				and not exists (select 1 from inboxes p where p.id = q.inbox_id and p.sending_paused_at is not null)
				and not exists (select 1 from organizations p where p.id = q.org_id and p.sending_paused_at is not null)
				order by next_attempt_at limit $1 for update skip locked)
			and i.id = m.inbox_id and d.id = i.domain_id
			returning m.id, m.org_id, m.inbox_id, i.address, m.to_addrs || m.cc_addrs || m.bcc_addrs, m.raw_key, m.attempts,
				d.name, d.dkim_selector, coalesce(d.dkim_private_key, ''), m.created_at`, n, sendLease.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j OutboundJob
			if err := rows.Scan(&j.ID, &j.OrgID, &j.InboxID, &j.Address, &j.Recipients, &j.RawKey, &j.Attempts,
				&j.DKIMDomain, &j.DKIMSel, &j.DKIMKeyPEM, &j.CreatedAt); err != nil {
				return err
			}
			jobs = append(jobs, j)
		}
		return rows.Err()
	})
	return jobs, err
}

// DefaultSendMaxAge is how long a message keeps being retried after a
// temporary failure: 5 days, in line with RFC 5321 section 4.5.4.1 and the
// Postfix default maximal_queue_lifetime, so a relay or remote MX outage of a
// few days does not lose mail.
const DefaultSendMaxAge = 120 * time.Hour

// sendSteps are the first retry delays; after them the sender retries every
// sendInterval (RFC 5321 asks for at least 30 minutes) until the max age.
var sendSteps = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour}

const sendInterval = 3 * time.Hour

// SendRetryDelay returns the wait after the given failed attempt (1 based).
func SendRetryDelay(attempt int) time.Duration {
	if attempt >= 1 && attempt <= len(sendSteps) {
		return sendSteps[attempt-1]
	}
	if attempt < 1 {
		return sendSteps[0]
	}
	return sendInterval
}

func (s *Service) sendMaxAge() time.Duration {
	if s.SendMaxAge > 0 {
		return s.SendMaxAge
	}
	return DefaultSendMaxAge
}

// outboundRetry decides what happens after a failed attempt: give up, or retry
// after the returned delay. The last retry is pulled in so it lands at the end
// of the window instead of past it.
func outboundRetry(j OutboundJob, sendErr error, now time.Time, maxAge time.Duration) (retry bool, delay time.Duration) {
	if errors.Is(sendErr, ErrPermanent) {
		return false, 0
	}
	left := j.CreatedAt.Add(maxAge).Sub(now)
	if left <= 0 {
		return false, 0
	}
	return true, min(SendRetryDelay(j.Attempts), left)
}

// FinishOutbound records the outcome of a delivery attempt and emits an event.
// Permanent (5xx) errors fail at once; temporary ones are retried until the
// message is older than the max age (MAI_SEND_MAX_AGE, default 5 days).
func (s *Service) FinishOutbound(ctx context.Context, j OutboundJob, sendErr error) error {
	retry, delay := false, time.Duration(0)
	if sendErr != nil {
		retry, delay = outboundRetry(j, sendErr, s.now(), s.sendMaxAge())
	}
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		switch {
		case sendErr == nil:
			if _, err := tx.Exec(ctx, "update messages set status = 'sent', last_error = null, next_attempt_at = null where id = $1", j.ID); err != nil {
				return err
			}
			if err := s.emitMessageEvent(ctx, tx, "message.sent", j.Address, j.ID); err != nil {
				return err
			}
			return s.finalOutcome(ctx, tx, j, "sent")
		case !retry:
			if _, err := tx.Exec(ctx, `update messages set status = 'failed', last_error = $2, next_attempt_at = null,
				labels = array(select distinct l from unnest(labels || '{failed}'::text[]) l) where id = $1`, j.ID, sendErr.Error()); err != nil {
				return err
			}
			if err := s.emitMessageEvent(ctx, tx, "message.failed", j.Address, j.ID); err != nil {
				return err
			}
			// Only a permanent (5xx) refusal is a hard bounce; giving up after
			// the retry window says nothing about the recipient address.
			if errors.Is(sendErr, ErrPermanent) {
				return s.finalOutcome(ctx, tx, j, "rejected")
			}
			return nil
		default:
			_, err := tx.Exec(ctx, "update messages set status = 'queued', last_error = $2, next_attempt_at = now() + $3::interval where id = $1",
				j.ID, sendErr.Error(), delay.String())
			return err
		}
	})
}

// finalOutcome records a send outcome and runs the bounce breaker.
func (s *Service) finalOutcome(ctx context.Context, tx pgx.Tx, j OutboundJob, outcome string) error {
	if j.InboxID == "" {
		return nil // a job built by hand (tests) without its inbox
	}
	if _, err := recordOutcome(ctx, tx, j.OrgID, j.InboxID, j.ID, outcome); err != nil {
		return err
	}
	return s.checkSendingHealth(ctx, tx, j.OrgID, j.InboxID)
}

// ErrPermanent marks a delivery error that must not be retried (5xx).
var ErrPermanent = errors.New("permanent delivery failure")

// FanOutEvents turns new events into one pending delivery per matching webhook.
func (s *Service) FanOutEvents(ctx context.Context, n int) (int, error) {
	count := 0
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id from events where fanned_out_at is null order by id limit $1 for update skip locked`, n)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if len(ids) == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `insert into webhook_deliveries (org_id, webhook_id, event_id)
			select e.org_id, w.id, e.id from events e join webhooks w on w.org_id = e.org_id
			where e.id = any($1) and w.enabled and e.type = any(w.event_types)
			and (cardinality(w.inbox_ids) = 0 or e.inbox_id = any(w.inbox_ids))
			on conflict do nothing`, ids); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "update events set fanned_out_at = now() where id = any($1)", ids)
		count = len(ids)
		return err
	})
	return count, err
}

type Delivery struct {
	ID        int64
	Attempt   int
	URL       string
	Secret    string
	EventID   string
	EventType string
	Payload   json.RawMessage
	CreatedAt time.Time
	seq       int64 // events.id, the outbox order
}

// ClaimDeliveries leases due webhook deliveries for one minute, oldest event
// first, so a first attempt reaches each endpoint in outbox order (for
// example message.security.completed before message.received). Retries can
// still arrive later than newer events.
func (s *Service) ClaimDeliveries(ctx context.Context, n int) ([]Delivery, error) {
	var list []Delivery
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `update webhook_deliveries d set attempt = attempt + 1, next_attempt_at = now() + interval '1 minute'
			from webhooks w, events e
			where d.id in (select id from webhook_deliveries where status = 'pending' and next_attempt_at <= now()
				order by next_attempt_at, event_id limit $1 for update skip locked)
			and w.id = d.webhook_id and e.id = d.event_id
			returning d.id, d.attempt, w.url, w.secret, e.event_id, e.type, e.payload, e.created_at, e.id`, n)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d Delivery
			if err := rows.Scan(&d.ID, &d.Attempt, &d.URL, &d.Secret, &d.EventID, &d.EventType, &d.Payload, &d.CreatedAt, &d.seq); err != nil {
				return err
			}
			list = append(list, d)
		}
		return rows.Err()
	})
	// update ... returning has no defined order; deliver in outbox order.
	slices.SortFunc(list, func(a, b Delivery) int { return cmp.Compare(a.seq, b.seq) })
	return list, err
}

// WebhookRetries follows the reference schedule: 5s, 5m, 30m, 2h, 5h, 10h, 10h.
var WebhookRetries = []time.Duration{5 * time.Second, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 5 * time.Hour, 10 * time.Hour, 10 * time.Hour}

func (s *Service) FinishDelivery(ctx context.Context, d Delivery, statusCode int, deliveryErr error) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		if deliveryErr == nil {
			_, err := tx.Exec(ctx, "update webhook_deliveries set status = 'succeeded', last_status_code = $2, last_error = null where id = $1", d.ID, statusCode)
			return err
		}
		if d.Attempt > len(WebhookRetries) {
			_, err := tx.Exec(ctx, "update webhook_deliveries set status = 'failed', last_status_code = $2, last_error = $3 where id = $1",
				d.ID, statusCode, deliveryErr.Error())
			return err
		}
		_, err := tx.Exec(ctx, "update webhook_deliveries set last_status_code = $2, last_error = $3, next_attempt_at = now() + $4::interval where id = $1",
			d.ID, statusCode, deliveryErr.Error(), WebhookRetries[d.Attempt-1].String())
		return err
	})
}

// EnrichJob is an inbound message waiting for the gatekeeper.
type EnrichJob struct {
	ID      string
	Address string
	Subject string
	Text    string
	HTML    string
	Auth    json.RawMessage
}

func (s *Service) ClaimEnrich(ctx context.Context, n int) ([]EnrichJob, error) {
	var jobs []EnrichJob
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		// A claim older than 10 minutes belongs to a crashed worker and is taken again.
		rows, err := tx.Query(ctx, `update messages m set gatekeeper = jsonb_build_object('status', 'pending', 'claimed_at', now())
			from inboxes i
			where m.id in (select id from messages where direction = 'in' and (gatekeeper is null
					or (gatekeeper->>'status' = 'pending' and (gatekeeper->>'claimed_at')::timestamptz < now() - interval '10 minutes'))
				order by created_at limit $1 for update skip locked)
			and i.id = m.inbox_id
			returning m.id, i.address, m.subject, coalesce(m.text_body, ''), coalesce(m.html_body, ''), m.auth_results`, n)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j EnrichJob
			if err := rows.Scan(&j.ID, &j.Address, &j.Subject, &j.Text, &j.HTML, &j.Auth); err != nil {
				return err
			}
			jobs = append(jobs, j)
		}
		return rows.Err()
	})
	return jobs, err
}

// Verdict is the gatekeeper output for one message.
type Verdict struct {
	Status        string   `json:"status"`
	Verdict       string   `json:"verdict"` // clear | review
	Score         float64  `json:"score"`
	Reasons       []string `json:"reasons"`
	Model         string   `json:"model"`
	ExtractedText string   `json:"-"`
}

// SaveEnrichment stores the verdict and emits the security events. For a
// message held by screen before notify it also decides the hold:
//   - clean verdict: message.security.completed, then message.received
//   - review verdict, or no verdict because the gatekeeper was unavailable
//     (fail closed): held for review, only message.security.review, redacted
//
// Messages that are not held keep the original behavior: completed, and
// review when flagged, with message.received already sent at ingest.
func (s *Service) SaveEnrichment(ctx context.Context, j EnrichJob, v Verdict) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var h heldMessage
		if err := tx.QueryRow(ctx, `select screening, thread_id, subject, preview, labels from messages where id = $1 for update`, j.ID).
			Scan(&h.screening, &h.threadID, &h.subject, &h.preview, &h.labels); err != nil {
			return err
		}
		pending := h.screening != nil && *h.screening == "pending"
		hold := pending && (v.Verdict != "clear" || v.Status != "complete")
		next := h.screening
		if pending {
			next = ptr("allowed")
			if hold {
				next = ptr("review")
			}
		}
		if _, err := tx.Exec(ctx, `update messages set gatekeeper = $2, extracted_text = nullif($3, ''), screening = $5,
			labels = case when $4 then array(select distinct l from unnest(labels || '{review}'::text[]) l) else labels end
			where id = $1`, j.ID, body, v.ExtractedText, v.Verdict == "review" || hold, next); err != nil {
			return err
		}
		switch {
		case hold:
			return s.emitEvent(ctx, tx, "message.security.review", j.Address, j.ID, true)
		case pending:
			if err := s.emitMessageEvent(ctx, tx, "message.security.completed", j.Address, j.ID); err != nil {
				return err
			}
			return s.notifyReceived(ctx, tx, j.Address, j.ID, h.threadID, h.subject, h.preview, h.labels)
		case h.screening != nil:
			// Held state already decided (screening timed out or the message
			// was released meanwhile): record the verdict, emit nothing more.
			return nil
		}
		if err := s.emitMessageEvent(ctx, tx, "message.security.completed", j.Address, j.ID); err != nil {
			return err
		}
		if v.Verdict == "review" {
			return s.emitMessageEvent(ctx, tx, "message.security.review", j.Address, j.ID)
		}
		return nil
	})
}
