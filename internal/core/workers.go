package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// OutboundJob is a queued message leased to the sender worker.
type OutboundJob struct {
	ID         string
	OrgID      string
	Address    string
	Recipients []string
	RawKey     string
	Attempts   int
	DKIMDomain string
	DKIMSel    string
	DKIMKeyPEM string
}

const sendLease = 10 * time.Minute

// ClaimOutbound leases up to n due messages. A lease that expires (crashed
// worker) makes the message due again.
func (s *Service) ClaimOutbound(ctx context.Context, n int) ([]OutboundJob, error) {
	var jobs []OutboundJob
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `update messages m set status = 'sending', attempts = attempts + 1,
				next_attempt_at = now() + $2::interval
			from inboxes i, domains d
			where m.id in (select id from messages where status in ('queued', 'sending') and next_attempt_at <= now()
				order by next_attempt_at limit $1 for update skip locked)
			and i.id = m.inbox_id and d.id = i.domain_id
			returning m.id, m.org_id, i.address, m.to_addrs || m.cc_addrs || m.bcc_addrs, m.raw_key, m.attempts,
				d.name, d.dkim_selector, coalesce(d.dkim_private_key, '')`, n, sendLease.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j OutboundJob
			if err := rows.Scan(&j.ID, &j.OrgID, &j.Address, &j.Recipients, &j.RawKey, &j.Attempts,
				&j.DKIMDomain, &j.DKIMSel, &j.DKIMKeyPEM); err != nil {
				return err
			}
			jobs = append(jobs, j)
		}
		return rows.Err()
	})
	return jobs, err
}

// SendRetries is the backoff schedule; after the last step the message fails.
var SendRetries = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}

// FinishOutbound records the outcome of a delivery attempt and emits an event.
func (s *Service) FinishOutbound(ctx context.Context, j OutboundJob, sendErr error) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		switch {
		case sendErr == nil:
			if _, err := tx.Exec(ctx, "update messages set status = 'sent', last_error = null, next_attempt_at = null where id = $1", j.ID); err != nil {
				return err
			}
			return s.emitMessageEvent(ctx, tx, "message.sent", j.Address, j.ID)
		case j.Attempts > len(SendRetries) || errors.Is(sendErr, ErrPermanent):
			if _, err := tx.Exec(ctx, `update messages set status = 'failed', last_error = $2, next_attempt_at = null,
				labels = array(select distinct l from unnest(labels || '{failed}'::text[]) l) where id = $1`, j.ID, sendErr.Error()); err != nil {
				return err
			}
			return s.emitMessageEvent(ctx, tx, "message.failed", j.Address, j.ID)
		default:
			delay := SendRetries[j.Attempts-1]
			_, err := tx.Exec(ctx, "update messages set status = 'queued', last_error = $2, next_attempt_at = now() + $3::interval where id = $1",
				j.ID, sendErr.Error(), delay.String())
			return err
		}
	})
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
}

// ClaimDeliveries leases due webhook deliveries for one minute.
func (s *Service) ClaimDeliveries(ctx context.Context, n int) ([]Delivery, error) {
	var list []Delivery
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `update webhook_deliveries d set attempt = attempt + 1, next_attempt_at = now() + interval '1 minute'
			from webhooks w, events e
			where d.id in (select id from webhook_deliveries where status = 'pending' and next_attempt_at <= now()
				order by next_attempt_at limit $1 for update skip locked)
			and w.id = d.webhook_id and e.id = d.event_id
			returning d.id, d.attempt, w.url, w.secret, e.event_id, e.type, e.payload, e.created_at`, n)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d Delivery
			if err := rows.Scan(&d.ID, &d.Attempt, &d.URL, &d.Secret, &d.EventID, &d.EventType, &d.Payload, &d.CreatedAt); err != nil {
				return err
			}
			list = append(list, d)
		}
		return rows.Err()
	})
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
		rows, err := tx.Query(ctx, `update messages m set gatekeeper = '{"status":"pending"}'
			from inboxes i
			where m.id in (select id from messages where direction = 'in' and gatekeeper is null
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

func (s *Service) SaveEnrichment(ctx context.Context, j EnrichJob, v Verdict) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `update messages set gatekeeper = $2, extracted_text = nullif($3, ''),
			labels = case when $4 then array(select distinct l from unnest(labels || '{review}'::text[]) l) else labels end
			where id = $1`, j.ID, body, v.ExtractedText, v.Verdict == "review"); err != nil {
			return err
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
