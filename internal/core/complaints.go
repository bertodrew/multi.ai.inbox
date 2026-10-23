package core

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/mailx"
)

// Complaints come from feedback loop reports (ARF, RFC 5965) that mailbox
// providers send when a recipient marks a message as spam. A report is
// matched to the outbound message by its Message-ID, which carries 96 random
// bits (mailx.NewMessageID): only the recipients of the message, and their
// providers, know it, and a recipient who forges a report is complaining
// anyway. Each message counts at most once (send_outcomes is unique per
// message and outcome), so one recipient cannot inflate the rate.

// recordComplaint handles a feedback report delivered to an inbox: when it
// is a complaint about a message this inbox sent, the message gets the
// complained label, a message.complained event and a complained outcome.
func (s *Service) recordComplaint(ctx context.Context, tx pgx.Tx, rc *LocalRecipient, raw []byte) (bool, error) {
	arf := mailx.ParseARF(raw)
	if arf == nil || !arf.Complaint() || arf.OriginalMessageID == "" {
		return false, nil
	}
	var msgID string
	err := tx.QueryRow(ctx, `select id from messages where inbox_id = $1 and rfc_message_id = $2 and direction = 'out' limit 1`,
		rc.InboxID, arf.OriginalMessageID).Scan(&msgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.complain(ctx, tx, rc.OrgID, rc.InboxID, rc.Address, msgID)
}

func (s *Service) complain(ctx context.Context, tx pgx.Tx, orgID, inboxID, address, msgID string) (bool, error) {
	added, err := recordOutcome(ctx, tx, orgID, inboxID, msgID, "complained")
	if err != nil || !added {
		return false, err
	}
	if _, err := tx.Exec(ctx, `update messages set labels = array(select distinct l from unnest(labels || '{complained}'::text[]) l)
		where id = $1`, msgID); err != nil {
		return false, err
	}
	return true, s.emitMessageEvent(ctx, tx, "message.complained", address, msgID)
}

// RecordFeedbackReport handles a report sent to the shared feedback loop
// address (MAI_FBL_ADDRESS), which belongs to no organization: the original
// message is looked up across all of them. The report itself is not stored.
// It returns whether a new complaint was recorded; the breaker runs in the
// same transaction.
func (s *Service) RecordFeedbackReport(ctx context.Context, raw []byte) (bool, error) {
	arf := mailx.ParseARF(raw)
	if arf == nil || !arf.Complaint() || arf.OriginalMessageID == "" {
		return false, nil
	}
	recorded := false
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var msgID, orgID, inboxID, address string
		err := tx.QueryRow(ctx, `select m.id, m.org_id, m.inbox_id, i.address from messages m join inboxes i on i.id = m.inbox_id
			where m.rfc_message_id = $1 and m.direction = 'out' limit 1`, arf.OriginalMessageID).Scan(&msgID, &orgID, &inboxID, &address)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // not ours, or the inbox was deleted since
		}
		if err != nil {
			return err
		}
		if recorded, err = s.complain(ctx, tx, orgID, inboxID, address, msgID); err != nil || !recorded {
			return err
		}
		return s.checkSendingHealth(ctx, tx, orgID, inboxID)
	})
	return recorded, err
}
