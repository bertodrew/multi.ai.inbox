package core

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// Screen before notify (doc 12, risk R08). With the organization option on,
// an inbound message moves through messages.screening:
//
//	pending  → allowed   clean verdict: message.security.completed, message.received
//	pending  → review    review verdict, gatekeeper unavailable or timeout:
//	                      message.security.review only, with bodies redacted
//	review   → released  privileged release: message.received
//
// Pending messages are never listed or readable. Review messages are readable
// only with include_review and the message_review permission.

// DefaultScreeningTimeout is how long a message may wait for the gatekeeper
// before it is held for review. It is longer than the 10 minute claim lease in
// ClaimEnrich, so a crashed enrich worker gets one retry first.
const DefaultScreeningTimeout = 15 * time.Minute

type heldMessage struct {
	screening *string
	threadID  string
	subject   string
	preview   string
	labels    []string
}

func ptr[T any](v T) *T { return &v }

// ReleaseMessage releases a message held for review: it becomes visible and
// message.received fires. The caller checks message_release and writes the
// audit entry.
func (s *Service) ReleaseMessage(ctx context.Context, orgID, address, id string) (*Message, error) {
	var out *Message
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		m, err := s.messageTx(ctx, tx, address, id, viewAll)
		if err != nil {
			return err
		}
		var h heldMessage
		if err := tx.QueryRow(ctx, `select screening, thread_id, subject, preview, labels from messages where id = $1 for update`, m.ID).
			Scan(&h.screening, &h.threadID, &h.subject, &h.preview, &h.labels); err != nil {
			return err
		}
		switch {
		case h.screening != nil && *h.screening == "pending":
			// Not found for readers, so do not confirm more than that it exists.
			return errf(http.StatusConflict, "not_held", "message is still being screened; release it once its verdict is review")
		case h.screening == nil || *h.screening != "review":
			return errf(http.StatusConflict, "not_held", "message is not held for review")
		}
		if _, err := tx.Exec(ctx, "update messages set screening = 'released' where id = $1", m.ID); err != nil {
			return err
		}
		// A human released it: if the inbox is an intake, the thread now opens
		// its ticket, the same way a message that screening allowed does.
		if err := s.IntakeTicketForMessage(ctx, tx, m.ID); err != nil {
			return err
		}
		if err := s.notifyReceived(ctx, tx, address, m.ID, h.threadID, h.subject, h.preview, h.labels); err != nil {
			return err
		}
		out, err = s.messageTx(ctx, tx, address, m.ID, viewDefault)
		return err
	})
	return out, err
}

// ExpireScreening holds for review every message that waited longer than
// after for its verdict (enrich role down or stuck). It fails closed: nothing
// is notified as received, and message.security.review fires with
// gatekeeper.status "timeout". It returns how many messages it moved.
func (s *Service) ExpireScreening(ctx context.Context, after time.Duration) (int, error) {
	count := 0
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `update messages m set screening = 'review',
				gatekeeper = jsonb_build_object('status', 'timeout', 'verdict', 'review', 'score', 0,
					'reasons', jsonb_build_array('screening_timeout'), 'model', 'none'),
				labels = array(select distinct l from unnest(m.labels || '{review}'::text[]) l)
			from inboxes i
			where m.id in (select id from messages where screening = 'pending' and created_at < now() - $1::interval
				order by created_at limit 100 for update skip locked)
			and i.id = m.inbox_id
			returning m.id, i.address`, after.String())
		if err != nil {
			return err
		}
		type held struct{ id, address string }
		var list []held
		for rows.Next() {
			var h held
			if err := rows.Scan(&h.id, &h.address); err != nil {
				rows.Close()
				return err
			}
			list = append(list, h)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, h := range list {
			if err := s.emitEvent(ctx, tx, "message.security.review", h.address, h.id, true); err != nil {
				return err
			}
		}
		count = len(list)
		return nil
	})
	return count, err
}
