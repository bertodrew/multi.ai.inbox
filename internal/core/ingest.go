package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/mailx"
)

// LocalRecipient is an inbox that accepts mail, resolved at RCPT time.
type LocalRecipient struct {
	InboxID string
	OrgID   string
	Address string
}

// ResolveRecipient looks up an address across all tenants (SMTP edge only).
func (s *Service) ResolveRecipient(ctx context.Context, address string) (*LocalRecipient, error) {
	r := &LocalRecipient{}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select id, org_id, address from inboxes where address = $1",
			strings.ToLower(strings.TrimSpace(address))).Scan(&r.InboxID, &r.OrgID, &r.Address)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// InboundMeta carries what the SMTP edge learned about a message.
type InboundMeta struct {
	MailFrom    string
	AuthResults map[string]any
	Labels      []string // e.g. "spam" from the spam filter verdict
}

// Ingest stores one inbound message for each local recipient, threads it and
// emits message.received. When the organization has screen_before_notify on,
// the message is stored as held (screening = pending) instead: it stays out of
// listings and thread counters, and the event waits for the gatekeeper verdict
// (see SaveEnrichment). The raw MIME is stored once per recipient.
func (s *Service) Ingest(ctx context.Context, raw []byte, rcpts []*LocalRecipient, meta InboundMeta) error {
	parsed, err := mailx.Parse(raw)
	if err != nil {
		return err
	}
	auth, _ := json.Marshal(meta.AuthResults)
	if meta.AuthResults == nil {
		auth = []byte("{}")
	}
	now := s.now()
	for _, rc := range rcpts {
		// Unique key per delivery: never derived from the sender controlled Message-ID,
		// so a duplicate Message-ID cannot overwrite an earlier message's raw source.
		rawKey := fmt.Sprintf("raw/%s/%s/%s-%s.eml", rc.OrgID, now.Format("2006/01"), rc.InboxID, newBlobID())
		if err := s.Blob.Put(ctx, rawKey, raw); err != nil {
			return fmt.Errorf("store raw: %w", err)
		}
		// Blobs written for this delivery; removed again unless the message row commits.
		written := []string{rawKey}
		inserted, bounced, complained := false, false, false
		err := s.DB.WithOrg(ctx, rc.OrgID, func(tx pgx.Tx) error {
			var dup bool
			if err := tx.QueryRow(ctx, "select exists(select 1 from messages where inbox_id = $1 and rfc_message_id = $2 and direction = 'in')",
				rc.InboxID, parsed.MessageID).Scan(&dup); err != nil {
				return err
			}
			if dup {
				return nil // a retried delivery of a message we already stored
			}
			var hold bool
			if err := tx.QueryRow(ctx, "select screen_before_notify from organizations where id = $1", rc.OrgID).Scan(&hold); err != nil {
				return err
			}
			var screening any
			if hold {
				screening = "pending"
			}
			threadID, err := s.findThread(ctx, tx, rc.InboxID, parsed)
			if err != nil {
				return err
			}
			preview := mailx.Preview(parsed.Text, 200)
			labels := uniqueLabels(append([]string{"received", "unread"}, meta.Labels...))
			newThread := threadID == ""
			if newThread {
				if err := tx.QueryRow(ctx, "insert into threads (org_id, inbox_id, subject, preview) values ($1, $2, $3, $4) returning id",
					rc.OrgID, rc.InboxID, parsed.Subject, preview).Scan(&threadID); err != nil {
					return err
				}
			}
			// A thread that opens in an intake inbox becomes a ticket in the same
			// transaction, so the queue and the mailbox cannot disagree.
			if newThread {
				if err := s.IntakeTicket(ctx, tx, rc.OrgID, rc.InboxID, threadID, parsed.Subject,
					strings.ToLower(parsed.From)); err != nil {
					return err
				}
			}
			var inReplyTo any
			if parsed.InReplyTo != "" {
				inReplyTo = parsed.InReplyTo
			}
			var msgID string
			if err := tx.QueryRow(ctx, `insert into messages (org_id, inbox_id, thread_id, direction, rfc_message_id, in_reply_to,
				refs, from_addr, to_addrs, cc_addrs, subject, text_body, html_body, preview, labels, raw_key, size_bytes,
				status, auth_results, screening)
				values ($1, $2, $3, 'in', $4, $5, $6, $7, $8, $9, $10, nullif($11, ''), nullif($12, ''), $13, $14, $15, $16, 'received', $17, $18)
				returning id`,
				rc.OrgID, rc.InboxID, threadID, parsed.MessageID, inReplyTo, nonNil(parsed.References), strings.ToLower(parsed.From),
				nonNil(parsed.To), nonNil(parsed.Cc), parsed.Subject, parsed.Text, parsed.HTML, preview, labels, rawKey, len(raw),
				auth, screening).Scan(&msgID); err != nil {
				return err
			}
			for i, a := range parsed.Attachments {
				key := fmt.Sprintf("att/%s/%s/%d", rc.OrgID, msgID, i)
				written = append(written, key)
				if err := s.Blob.Put(ctx, key, a.Data); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `insert into attachments (org_id, message_id, filename, content_type, size_bytes, storage_key)
					values ($1, $2, $3, $4, $5, $6)`, rc.OrgID, msgID, a.Filename, a.ContentType, len(a.Data), key); err != nil {
					return err
				}
			}
			inserted = true
			// A DSN counts against the sending inbox whether or not it is held.
			if bounced, err = s.recordBounce(ctx, tx, rc, parsed, raw, meta); err != nil {
				return err
			}
			// So does a feedback loop complaint (ARF).
			if complained, err = s.recordComplaint(ctx, tx, rc, raw); err != nil {
				return err
			}
			if meta.AuthResults["antivirus"] == "unscanned" {
				source := "mx"
				if meta.AuthResults["internal"] == true {
					source = "internal"
				}
				if err := s.alertAVUnscanned(ctx, tx, rc.OrgID, rc.InboxID, rc.Address, source); err != nil {
					return err
				}
			}
			if hold {
				return nil // counted and notified once screened or released
			}
			return s.notifyReceived(ctx, tx, rc.Address, msgID, threadID, parsed.Subject, preview, labels)
		})
		if err != nil || !inserted {
			// A duplicate delivery or a rolled back transaction: nothing references these blobs.
			for _, key := range written {
				_ = s.Blob.Delete(ctx, key)
			}
		}
		if err != nil {
			return err
		}
		if bounced || complained {
			if err := s.CheckSendingHealth(ctx, rc.OrgID, rc.InboxID); err != nil {
				return fmt.Errorf("sending health: %w", err)
			}
		}
	}
	return nil
}

// findThread joins a reply to the thread of the message it references in the same inbox.
func (s *Service) findThread(ctx context.Context, tx pgx.Tx, inboxID string, p *mailx.Parsed) (string, error) {
	ids := append([]string{}, p.References...)
	if p.InReplyTo != "" {
		ids = append(ids, p.InReplyTo)
	}
	if len(ids) == 0 {
		return "", nil
	}
	var threadID string
	err := tx.QueryRow(ctx, `select thread_id from messages where inbox_id = $1 and rfc_message_id = any($2)
		order by created_at desc limit 1`, inboxID, ids).Scan(&threadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return threadID, err
}

// notifyReceived makes an inbound message visible in its thread and emits
// message.received (or message.received.spam).
func (s *Service) notifyReceived(ctx context.Context, tx pgx.Tx, address, msgID, threadID, subject, preview string, labels []string) error {
	if err := touchThread(ctx, tx, threadID, subject, preview, labels); err != nil {
		return err
	}
	eventType := "message.received"
	if contains(labels, "spam") {
		eventType = "message.received.spam"
	}
	return s.emitMessageEvent(ctx, tx, eventType, address, msgID)
}

// emitMessageEvent writes an outbox event with the full message and its thread.
func (s *Service) emitMessageEvent(ctx context.Context, tx pgx.Tx, eventType, address, msgID string) error {
	return s.emitEvent(ctx, tx, eventType, address, msgID, false)
}

// emitEvent writes an outbox event; redact drops every field that carries
// body content (text, html, extracted_text, previews), so an event about a
// held message cannot deliver its prompt injection to a webhook consumer.
func (s *Service) emitEvent(ctx context.Context, tx pgx.Tx, eventType, address, msgID string, redact bool) error {
	m, err := s.messageTx(ctx, tx, address, msgID, viewAll)
	if err != nil {
		return err
	}
	t, err := scanThread(tx.QueryRow(ctx, "select "+threadCols+threadFrom+" where t.id = $1", m.ThreadID))
	if err != nil {
		return err
	}
	if redact {
		m.Text, m.HTML, m.ExtractedText, m.Preview = nil, nil, nil, ""
		t.Preview = ""
	}
	return insertEvent(ctx, tx, m.orgID, m.inboxUUID, eventType, map[string]any{"message": m, "thread": t})
}

func insertEvent(ctx context.Context, tx pgx.Tx, orgID, inboxID, eventType string, data map[string]any) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var inbox any
	if inboxID != "" {
		inbox = inboxID
	}
	_, err = tx.Exec(ctx, "insert into events (org_id, inbox_id, type, payload) values ($1, $2, $3, $4)", orgID, inbox, eventType, body)
	return err
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func newBlobID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
