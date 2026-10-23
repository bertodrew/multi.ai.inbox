package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Drafts are messages an agent prepares and sends later (doc 05, Draft). A
// draft is sent through the same path as POST .../messages/send, so plan
// limits, the sending pause and idempotency apply, and sending deletes it.

// DraftAttachment describes an attachment of a draft; the content is only
// returned once the draft is sent and becomes a message.
type DraftAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

type Draft struct {
	ID          string            `json:"draft_id"`
	InboxID     string            `json:"inbox_id"`
	ThreadID    *string           `json:"thread_id,omitempty"`
	InReplyTo   *string           `json:"in_reply_to,omitempty"` // RFC Message-ID of the message replied to
	References  []string          `json:"references"`
	To          []string          `json:"to"`
	Cc          []string          `json:"cc"`
	Bcc         []string          `json:"bcc"`
	Subject     string            `json:"subject"`
	Text        string            `json:"text"`
	HTML        string            `json:"html"`
	Labels      []string          `json:"labels"`
	Attachments []DraftAttachment `json:"attachments"`
	SendAt      *time.Time        `json:"send_at,omitempty"`
	Status      string            `json:"status"` // draft | scheduled | failed
	LastError   *string           `json:"last_error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	atts        []AttachmentInput
	orgID       string
	inboxUUID   string
}

// CreateDraftRequest creates a draft. With in_reply_to the draft is a reply:
// to, subject and threading default to those of the message replied to, as
// for POST .../messages/{message}/reply.
type CreateDraftRequest struct {
	To          []string          `json:"to"`
	Cc          []string          `json:"cc"`
	Bcc         []string          `json:"bcc"`
	Subject     string            `json:"subject"`
	Text        string            `json:"text"`
	HTML        string            `json:"html"`
	Labels      []string          `json:"labels"`
	Attachments []AttachmentInput `json:"attachments"`
	InReplyTo   string            `json:"in_reply_to"` // message id or RFC Message-ID
	ReplyAll    bool              `json:"reply_all"`
	SendAt      *time.Time        `json:"send_at"`
}

// UpdateDraftRequest replaces the fields it carries. send_at null cancels a
// scheduled send. The reply target is fixed at create.
type UpdateDraftRequest struct {
	To          *[]string          `json:"to"`
	Cc          *[]string          `json:"cc"`
	Bcc         *[]string          `json:"bcc"`
	Subject     *string            `json:"subject"`
	Text        *string            `json:"text"`
	HTML        *string            `json:"html"`
	Labels      *[]string          `json:"labels"`
	Attachments *[]AttachmentInput `json:"attachments"`
	SendAt      NullableTime       `json:"send_at"`
}

// NullableTime tells an absent JSON field (Set false) from an explicit null.
type NullableTime struct {
	Set  bool
	Time *time.Time
}

func (n *NullableTime) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Time = nil
		return nil
	}
	var t time.Time
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}
	n.Time = &t
	return nil
}

const maxScheduleAhead = 365 * 24 * time.Hour

const draftCols = `d.id, i.address, d.thread_id, d.in_reply_to, d.refs, d.to_addrs, d.cc_addrs, d.bcc_addrs, d.subject,
	d.text_body, d.html_body, d.labels, d.attachments, d.send_at, d.status, d.last_error, d.created_at, d.updated_at,
	d.org_id, d.inbox_id`

const draftFrom = " from drafts d join inboxes i on i.id = d.inbox_id"

func scanDraft(row pgx.Row) (*Draft, error) {
	d := &Draft{}
	var atts []byte
	err := row.Scan(&d.ID, &d.InboxID, &d.ThreadID, &d.InReplyTo, &d.References, &d.To, &d.Cc, &d.Bcc, &d.Subject,
		&d.Text, &d.HTML, &d.Labels, &atts, &d.SendAt, &d.Status, &d.LastError, &d.CreatedAt, &d.UpdatedAt,
		&d.orgID, &d.inboxUUID)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(atts, &d.atts); err != nil {
		return nil, fmt.Errorf("draft attachments: %w", err)
	}
	d.Attachments = draftAttachments(d.atts)
	return d, nil
}

func draftAttachments(in []AttachmentInput) []DraftAttachment {
	out := make([]DraftAttachment, 0, len(in))
	for _, a := range in {
		out = append(out, DraftAttachment{Filename: a.Filename, ContentType: a.ContentType,
			Size: base64.StdEncoding.DecodedLen(len(a.Content)) - padding(a.Content)})
	}
	return out
}

func padding(b64 string) int {
	n := 0
	for i := len(b64) - 1; i >= 0 && b64[i] == '='; i-- {
		n++
	}
	return n
}

// check validates the draft as it would be stored. Recipients and a body are
// only required to send, so an agent can build a draft in steps.
func (d *Draft) check(now time.Time, sendAtChanged bool) error {
	var err error
	if d.To, err = normalizeAddrs(d.To); err != nil {
		return err
	}
	if d.Cc, err = normalizeAddrs(d.Cc); err != nil {
		return err
	}
	if d.Bcc, err = normalizeAddrs(d.Bcc); err != nil {
		return err
	}
	if n := len(d.To) + len(d.Cc) + len(d.Bcc); n > maxRecipients {
		return Validation("at most %d recipients per message, got %d", maxRecipients, n)
	}
	if len(d.Subject) > 998 {
		return Validation("subject is longer than 998 characters")
	}
	d.Labels = uniqueLabels(d.Labels)
	if err := validateCustomLabels(d.Labels); err != nil {
		return err
	}
	if _, err := decodeAttachments(d.atts); err != nil {
		return err
	}
	if d.atts == nil {
		d.atts = []AttachmentInput{}
	}
	d.Attachments = draftAttachments(d.atts)
	if sendAtChanged && d.SendAt != nil {
		if !d.SendAt.After(now) {
			return Validation("send_at must be in the future")
		}
		if d.SendAt.After(now.Add(maxScheduleAhead)) {
			return Validation("send_at must be within 365 days")
		}
	}
	return nil
}

// status follows send_at: scheduled while a send time is set, draft otherwise.
func (d *Draft) schedule() {
	d.Status, d.LastError = "draft", nil
	if d.SendAt != nil {
		d.Status = "scheduled"
	}
}

func (s *Service) CreateDraft(ctx context.Context, orgID, address string, req CreateDraftRequest) (*Draft, error) {
	d := &Draft{To: req.To, Cc: req.Cc, Bcc: req.Bcc, Subject: req.Subject, Text: req.Text, HTML: req.HTML,
		Labels: req.Labels, atts: req.Attachments, SendAt: req.SendAt, References: []string{}}
	var out *Draft
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return err
		}
		if req.InReplyTo != "" {
			// As for a reply: a held message must not steer the agent.
			parent, err := s.messageTx(ctx, tx, address, req.InReplyTo, viewDefault)
			if err != nil {
				return err
			}
			o := replyOutbound(inbox, parent, req.ReplyAll)
			if len(d.To) == 0 {
				d.To = o.to
			}
			if len(d.Cc) == 0 {
				d.Cc = o.cc
			}
			if d.Subject == "" {
				d.Subject = o.subject
			}
			d.ThreadID, d.InReplyTo, d.References = &o.threadID, &o.inReplyTo, o.references
		} else if req.ReplyAll {
			return Validation("reply_all needs in_reply_to")
		}
		if err := d.check(s.now(), true); err != nil {
			return err
		}
		d.schedule()
		atts, _ := json.Marshal(d.atts)
		var id string
		if err := tx.QueryRow(ctx, `insert into drafts (org_id, inbox_id, thread_id, in_reply_to, refs, to_addrs, cc_addrs,
			bcc_addrs, subject, text_body, html_body, labels, attachments, send_at, status)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) returning id`,
			orgID, inbox.ID, d.ThreadID, d.InReplyTo, d.References, nonNil(d.To), nonNil(d.Cc), nonNil(d.Bcc), d.Subject,
			d.Text, d.HTML, d.Labels, atts, d.SendAt, d.Status).Scan(&id); err != nil {
			return err
		}
		if out, err = s.draftTx(ctx, tx, inbox, id, false); err != nil {
			return err
		}
		return insertEvent(ctx, tx, orgID, inbox.ID, "draft.created", map[string]any{"draft": out})
	})
	return out, err
}

// draftTx loads a draft of inbox; lock takes the row for update.
func (s *Service) draftTx(ctx context.Context, tx pgx.Tx, inbox *Inbox, id string, lock bool) (*Draft, error) {
	q := "select " + draftCols + draftFrom + " where d.inbox_id = $1 and d.id::text = $2"
	if lock {
		q += " for update of d"
	}
	d, err := scanDraft(tx.QueryRow(ctx, q, inbox.ID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NotFound("draft")
	}
	return d, err
}

func (s *Service) GetDraft(ctx context.Context, orgID, address, id string) (*Draft, error) {
	var out *Draft
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return err
		}
		out, err = s.draftTx(ctx, tx, inbox, id, false)
		return err
	})
	return out, err
}

// ListDrafts returns the drafts of an inbox, newest first.
func (s *Service) ListDrafts(ctx context.Context, orgID, address string, p Page) ([]*Draft, string, error) {
	cur, err := p.cursor()
	if err != nil {
		return nil, "", err
	}
	var list []*Draft
	err = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return err
		}
		q := "select " + draftCols + draftFrom + " where d.inbox_id = $1"
		args := []any{inbox.ID, p.limit() + 1}
		if cur != nil {
			args = append(args, cur.At, cur.ID)
			q += " and (d.created_at, d.id) < ($3, $4)"
		}
		rows, err := tx.Query(ctx, q+" order by d.created_at desc, d.id desc limit $2", args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDraft(rows)
			if err != nil {
				return err
			}
			list = append(list, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(list) > p.limit() {
		list = list[:p.limit()]
		last := list[len(list)-1]
		next = nextToken(last.CreatedAt, last.ID)
	}
	return list, next, nil
}

func (s *Service) UpdateDraft(ctx context.Context, orgID, address, id string, req UpdateDraftRequest) (*Draft, error) {
	var out *Draft
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return err
		}
		d, err := s.draftTx(ctx, tx, inbox, id, true)
		if err != nil {
			return err
		}
		set := func(dst *[]string, v *[]string) {
			if v != nil {
				*dst = *v
			}
		}
		set(&d.To, req.To)
		set(&d.Cc, req.Cc)
		set(&d.Bcc, req.Bcc)
		set(&d.Labels, req.Labels)
		if req.Subject != nil {
			d.Subject = *req.Subject
		}
		if req.Text != nil {
			d.Text = *req.Text
		}
		if req.HTML != nil {
			d.HTML = *req.HTML
		}
		if req.Attachments != nil {
			d.atts = *req.Attachments
		}
		switch {
		case req.SendAt.Set:
			d.SendAt = req.SendAt.Time
			d.schedule()
		case d.Status == "failed":
			// An edit after a refused scheduled send makes it a plain draft
			// again; set send_at to schedule it anew.
			d.SendAt = nil
			d.schedule()
		}
		if err := d.check(s.now(), req.SendAt.Set); err != nil {
			return err
		}
		atts, _ := json.Marshal(d.atts)
		if _, err := tx.Exec(ctx, `update drafts set to_addrs = $2, cc_addrs = $3, bcc_addrs = $4, subject = $5, text_body = $6,
			html_body = $7, labels = $8, attachments = $9, send_at = $10, status = $11, last_error = $12, updated_at = now()
			where id = $1`, d.ID, nonNil(d.To), nonNil(d.Cc), nonNil(d.Bcc), d.Subject, d.Text, d.HTML, d.Labels, atts,
			d.SendAt, d.Status, d.LastError); err != nil {
			return err
		}
		out, err = s.draftTx(ctx, tx, inbox, d.ID, false)
		return err
	})
	return out, err
}

func (s *Service) DeleteDraft(ctx context.Context, orgID, address, id string) error {
	return s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "delete from drafts where inbox_id = $1 and id::text = $2", inbox.ID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return NotFound("draft")
		}
		return nil
	})
}

// SendDraft sends a draft now, whatever its send_at, and deletes it. A
// repeated Idempotency-Key returns the first result even though the draft is
// gone by then.
func (s *Service) SendDraft(ctx context.Context, orgID, address, id, idemKey string) (*SendResult, error) {
	req := struct {
		Inbox string `json:"inbox"`
		Draft string `json:"draft_id"`
	}{address, id}
	return s.idempotent(ctx, orgID, idemKey, req, func(tx pgx.Tx) (*SendResult, error) {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return nil, err
		}
		d, err := s.draftTx(ctx, tx, inbox, id, true)
		if err != nil {
			return nil, err
		}
		return s.sendDraftTx(ctx, tx, inbox, d)
	})
}

// sendDraftTx queues the draft as an outbound message and deletes it.
func (s *Service) sendDraftTx(ctx context.Context, tx pgx.Tx, inbox *Inbox, d *Draft) (*SendResult, error) {
	if len(d.To) == 0 {
		return nil, Validation("at least one recipient in to is required")
	}
	if d.Text == "" && d.HTML == "" {
		return nil, Validation("text or html is required")
	}
	atts, err := decodeAttachments(d.atts)
	if err != nil {
		return nil, err
	}
	o := outbound{to: d.To, cc: d.Cc, bcc: d.Bcc, subject: d.Subject, text: d.Text, html: d.HTML, labels: d.Labels,
		attachments: atts, references: d.References}
	if d.ThreadID != nil {
		o.threadID = *d.ThreadID
	}
	if d.InReplyTo != nil {
		o.inReplyTo = *d.InReplyTo
	}
	res, err := s.queue(ctx, tx, d.orgID, inbox, o)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "delete from drafts where id = $1", d.ID); err != nil {
		return nil, err
	}
	return res, nil
}

// SendDueDrafts sends up to n scheduled drafts whose send_at has passed.
// A send the platform refuses (plan limit, sending paused, no recipient)
// marks the draft failed with the reason instead of retrying every minute.
// It returns how many drafts it sent and how many failed.
func (s *Service) SendDueDrafts(ctx context.Context, n int) (sent, failed int, err error) {
	type due struct{ id, orgID, address string }
	var list []due
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select d.id, d.org_id, i.address from drafts d join inboxes i on i.id = d.inbox_id
			where d.status = 'scheduled' and d.send_at <= now() order by d.send_at limit $1`, n)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.orgID, &d.address); err != nil {
				return err
			}
			list = append(list, d)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, 0, err
	}
	var firstErr error
	for _, item := range list {
		var gone bool
		sendErr := s.DB.WithOrg(ctx, item.orgID, func(tx pgx.Tx) error {
			inbox, err := s.inboxTx(ctx, tx, item.address)
			if err != nil {
				return err
			}
			// skip locked: a manual send or an edit holds the row right now.
			d, err := scanDraft(tx.QueryRow(ctx, "select "+draftCols+draftFrom+` where d.id = $1
				and d.status = 'scheduled' and d.send_at <= now() for update of d skip locked`, item.id))
			if errors.Is(err, pgx.ErrNoRows) {
				gone = true
				return nil
			}
			if err != nil {
				return err
			}
			_, err = s.sendDraftTx(ctx, tx, inbox, d)
			return err
		})
		var refused *Error
		switch {
		case sendErr == nil && gone:
		case sendErr == nil:
			sent++
		case errors.As(sendErr, &refused) && refused.Status < 500:
			failed++
			if err := s.DB.WithOrg(ctx, item.orgID, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `update drafts set status = 'failed', last_error = $2, updated_at = now()
					where id = $1 and status = 'scheduled'`, item.id, refused.Message)
				return err
			}); err != nil && firstErr == nil {
				firstErr = err
			}
		default:
			if firstErr == nil {
				firstErr = fmt.Errorf("send draft %s: %w", item.id, sendErr)
			}
		}
	}
	return sent, failed, firstErr
}
