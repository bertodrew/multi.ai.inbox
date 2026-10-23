package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/mailx"
)

// Sending health breaker (doc 11: pause above 5% bounces). Over the rolling
// window, once at least HealthMinSends messages reached a final outcome,
// a hard bounce rate above HealthMaxBounceRate pauses sending for the inbox,
// and the same rule over all the organization's sends pauses the
// organization. Hard bounces are permanent 5xx failures at send time plus
// failed delivery status notifications received later.
//
// Complaints (feedback loop reports) trip the same breaker: above
// HealthMaxComplaintRate once at least HealthMinComplaintSends messages were
// sent, or HealthSmallSenderComplaints complaints in the window for senders
// below that volume, where a rate means little.
const (
	HealthWindow        = 24 * time.Hour
	HealthMinSends      = 20
	HealthMaxBounceRate = 0.05

	HealthMinComplaintSends     = 1000
	HealthMaxComplaintRate      = 0.001
	HealthSmallSenderComplaints = 3
)

type healthStats struct {
	sends      int // final outcomes: sent + rejected
	hard       int // rejected + bounced
	complaints int // complained
}

func (h healthStats) bounceTripped() bool {
	return h.sends >= HealthMinSends && float64(h.hard) > HealthMaxBounceRate*float64(h.sends)
}

func (h healthStats) complaintTripped() bool {
	if h.sends >= HealthMinComplaintSends {
		return float64(h.complaints) > HealthMaxComplaintRate*float64(h.sends)
	}
	return h.complaints >= HealthSmallSenderComplaints
}

func (h healthStats) tripped() bool { return h.bounceTripped() || h.complaintTripped() }

func (h healthStats) reason() string {
	window := strings.TrimSuffix(HealthWindow.String(), "0m0s")
	switch {
	case h.bounceTripped():
		return fmt.Sprintf("hard bounce rate %.1f%% (%d of %d sends in the last %s) is above %.0f%%",
			100*float64(h.hard)/float64(h.sends), h.hard, h.sends, window, 100*HealthMaxBounceRate)
	case h.sends >= HealthMinComplaintSends:
		return fmt.Sprintf("complaint rate %.2f%% (%d of %d sends in the last %s) is above %.1f%%",
			100*float64(h.complaints)/float64(h.sends), h.complaints, h.sends, window, 100*HealthMaxComplaintRate)
	}
	return fmt.Sprintf("%d spam complaints in the last %s (%d sends; below %d sends the limit is %d complaints)",
		h.complaints, window, h.sends, HealthMinComplaintSends, HealthSmallSenderComplaints)
}

// recordOutcome stores the final outcome of an outbound message once.
func recordOutcome(ctx context.Context, tx pgx.Tx, orgID, inboxID, messageID, outcome string) (bool, error) {
	tag, err := tx.Exec(ctx, `insert into send_outcomes (org_id, inbox_id, message_id, outcome) values ($1, $2, $3, $4)
		on conflict (message_id, outcome) do nothing`, orgID, inboxID, messageID, outcome)
	return tag.RowsAffected() == 1, err
}

const statsQuery = `select count(*) filter (where outcome in ('sent', 'rejected')),
		count(*) filter (where outcome in ('rejected', 'bounced')),
		count(*) filter (where outcome = 'complained')
	from send_outcomes where %s = $1 and created_at > greatest(now() - $2::interval, coalesce($3, '-infinity'::timestamptz))`

// checkSendingHealth evaluates the breaker for an inbox and its organization
// and pauses whichever trips, emitting inbox.sending.paused or
// organization.sending.paused. It needs the system context (organizations
// are writable only there).
func (s *Service) checkSendingHealth(ctx context.Context, tx pgx.Tx, orgID, inboxID string) error {
	var address string
	var inboxPaused bool
	var inboxResumed *time.Time
	err := tx.QueryRow(ctx, "select address, sending_paused_at is not null, sending_resumed_at from inboxes where id = $1",
		inboxID).Scan(&address, &inboxPaused, &inboxResumed)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && !inboxPaused {
		var st healthStats
		if err := tx.QueryRow(ctx, fmt.Sprintf(statsQuery, "inbox_id"), inboxID, HealthWindow.String(), inboxResumed).Scan(&st.sends, &st.hard, &st.complaints); err != nil {
			return err
		}
		if st.tripped() {
			tag, err := tx.Exec(ctx, `update inboxes set sending_paused_at = now(), sending_paused_reason = $2
				where id = $1 and sending_paused_at is null`, inboxID, st.reason())
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				if err := insertEvent(ctx, tx, orgID, inboxID, "inbox.sending.paused", map[string]any{
					"inbox_id": address, "reason": st.reason(), "sends": st.sends, "hard_bounces": st.hard, "complaints": st.complaints}); err != nil {
					return err
				}
			}
		}
	}
	var orgPaused bool
	var orgResumed *time.Time
	if err := tx.QueryRow(ctx, "select sending_paused_at is not null, sending_resumed_at from organizations where id = $1",
		orgID).Scan(&orgPaused, &orgResumed); err != nil {
		return err
	}
	if orgPaused {
		return nil
	}
	var st healthStats
	if err := tx.QueryRow(ctx, fmt.Sprintf(statsQuery, "org_id"), orgID, HealthWindow.String(), orgResumed).Scan(&st.sends, &st.hard, &st.complaints); err != nil {
		return err
	}
	if !st.tripped() {
		return nil
	}
	tag, err := tx.Exec(ctx, `update organizations set sending_paused_at = now(), sending_paused_reason = $2
		where id = $1 and sending_paused_at is null`, orgID, st.reason())
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	return insertEvent(ctx, tx, orgID, "", "organization.sending.paused", map[string]any{
		"organization_id": orgID, "reason": st.reason(), "sends": st.sends, "hard_bounces": st.hard, "complaints": st.complaints})
}

// sendingPaused refuses a send when the inbox or its organization is paused.
func sendingPaused(ctx context.Context, tx pgx.Tx, orgID, inboxID, address string) error {
	var inboxReason, orgReason *string
	if err := tx.QueryRow(ctx, `select
		(select coalesce(sending_paused_reason, '') from inboxes where id = $1 and sending_paused_at is not null),
		(select coalesce(sending_paused_reason, '') from organizations where id = $2 and sending_paused_at is not null)`,
		inboxID, orgID).Scan(&inboxReason, &orgReason); err != nil {
		return err
	}
	if orgReason != nil {
		return errf(http.StatusForbidden, "sending_paused", "sending is paused for this organization: %s; contact support to resume", *orgReason)
	}
	if inboxReason != nil {
		return errf(http.StatusForbidden, "sending_paused", "sending from %s is paused: %s; contact support to resume", address, *inboxReason)
	}
	return nil
}

// recordBounce handles a delivery status notification sent to an inbox: when
// it reports a permanent failure for a message this inbox sent, the message
// gets the bounced label, a message.bounced event and a bounced outcome.
// Only null sender mail (MAIL FROM:<>, RFC 5321 section 4.5.5) counts, and
// the failed recipient must be one the message was sent to, so ordinary mail
// cannot forge bounces against an inbox.
func (s *Service) recordBounce(ctx context.Context, tx pgx.Tx, rc *LocalRecipient, parsed *mailx.Parsed, raw []byte, meta InboundMeta) (bool, error) {
	if meta.MailFrom != "" {
		return false, nil
	}
	dsn := mailx.ParseDSN(raw)
	if dsn == nil || len(dsn.Failed) == 0 {
		return false, nil
	}
	original := dsn.OriginalMessageID
	if original == "" {
		original = parsed.InReplyTo
	}
	if original == "" {
		return false, nil
	}
	var msgID string
	var rcpts []string
	err := tx.QueryRow(ctx, `select id, to_addrs || cc_addrs || bcc_addrs from messages
		where inbox_id = $1 and rfc_message_id = $2 and direction = 'out' limit 1`, rc.InboxID, original).Scan(&msgID, &rcpts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var failed []mailx.DSNRecipient
	for _, f := range dsn.Failed {
		if contains(rcpts, f.Address) {
			failed = append(failed, f)
		}
	}
	if len(failed) == 0 {
		return false, nil
	}
	added, err := recordOutcome(ctx, tx, rc.OrgID, rc.InboxID, msgID, "bounced")
	if err != nil || !added {
		return false, err
	}
	if _, err := tx.Exec(ctx, `update messages set labels = array(select distinct l from unnest(labels || '{bounced}'::text[]) l),
		last_error = $2 where id = $1`, msgID, "bounced: "+failed[0].Status+" "+failed[0].Diagnostic); err != nil {
		return false, err
	}
	return true, s.emitMessageEvent(ctx, tx, "message.bounced", rc.Address, msgID)
}

// CheckSendingHealth evaluates the breaker after an outcome recorded outside
// the system context (a bounce received over SMTP).
func (s *Service) CheckSendingHealth(ctx context.Context, orgID, inboxID string) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error { return s.checkSendingHealth(ctx, tx, orgID, inboxID) })
}

// ResumeInboxSending clears a paused inbox (operator command) and starts a
// fresh window, emitting inbox.sending.resumed.
func (s *Service) ResumeInboxSending(ctx context.Context, address string) (*Inbox, error) {
	var out *Inbox
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var orgID string
		var err error
		out, err = scanInbox(tx.QueryRow(ctx, `update inboxes set sending_paused_at = null, sending_paused_reason = null,
			sending_resumed_at = now() where address = $1 returning `+inboxCols, strings.ToLower(strings.TrimSpace(address))))
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("inbox")
		}
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "select org_id from inboxes where id = $1", out.ID).Scan(&orgID); err != nil {
			return err
		}
		return insertEvent(ctx, tx, orgID, out.ID, "inbox.sending.resumed", map[string]any{"inbox_id": out.InboxID})
	})
	return out, err
}

// ResumeOrgSending clears a paused organization (operator command).
func (s *Service) ResumeOrgSending(ctx context.Context, orgID string) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `update organizations set sending_paused_at = null, sending_paused_reason = null,
			sending_resumed_at = now() where id = $1`, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return NotFound("organization")
		}
		return insertEvent(ctx, tx, orgID, "", "organization.sending.resumed", map[string]any{"organization_id": orgID})
	})
}
