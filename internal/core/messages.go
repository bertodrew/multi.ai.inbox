package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/mailx"
)

type AttachmentInfo struct {
	ID          string `json:"attachment_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

type Message struct {
	ID            string           `json:"id"`
	MessageID     string           `json:"message_id"` // RFC 5322 Message-ID
	ThreadID      string           `json:"thread_id"`
	InboxID       string           `json:"inbox_id"`
	Direction     string           `json:"direction"`
	From          string           `json:"from"`
	To            []string         `json:"to"`
	Cc            []string         `json:"cc"`
	Bcc           []string         `json:"bcc,omitempty"`
	Subject       string           `json:"subject"`
	Text          *string          `json:"text,omitempty"`
	HTML          *string          `json:"html,omitempty"`
	ExtractedText *string          `json:"extracted_text,omitempty"`
	Preview       string           `json:"preview"`
	Labels        []string         `json:"labels"`
	InReplyTo     *string          `json:"in_reply_to,omitempty"`
	References    []string         `json:"references"`
	Status        string           `json:"status"`
	Size          int              `json:"size"`
	AuthResults   json.RawMessage  `json:"authentication_results"`
	Gatekeeper    json.RawMessage  `json:"gatekeeper,omitempty"`
	Screening     *string          `json:"screening,omitempty"` // pending | review | allowed | released
	Attachments   []AttachmentInfo `json:"attachments"`
	CreatedAt     time.Time        `json:"created_at"`
	rawKey        *string
	orgID         string
	inboxUUID     string
}

const messageCols = `m.id, m.rfc_message_id, m.thread_id, i.address, m.direction, m.from_addr, m.to_addrs, m.cc_addrs,
	m.bcc_addrs, m.subject, m.text_body, m.html_body, m.extracted_text, m.preview, m.labels, m.in_reply_to, m.refs,
	m.status, m.size_bytes, m.auth_results, m.gatekeeper, m.screening, m.created_at, m.raw_key, m.org_id, m.inbox_id`

const messageFrom = " from messages m join inboxes i on i.id = m.inbox_id"

// view says which held messages a read may see (screen before notify).
type view int

const (
	viewDefault view = iota // hides pending and review
	viewReview              // hides pending only (include_review with message_review)
	viewAll                 // internal: events, release
)

func viewFor(includeReview bool) view {
	if includeReview {
		return viewReview
	}
	return viewDefault
}

// visible returns the SQL condition on messages alias m for v.
func (v view) visible() string {
	switch v {
	case viewReview:
		return "m.screening is distinct from 'pending'"
	case viewAll:
		return "true"
	}
	return "(m.screening is null or m.screening not in ('pending', 'review'))"
}

func scanMessage(row pgx.Row) (*Message, error) {
	m := &Message{}
	err := row.Scan(&m.ID, &m.MessageID, &m.ThreadID, &m.InboxID, &m.Direction, &m.From, &m.To, &m.Cc, &m.Bcc,
		&m.Subject, &m.Text, &m.HTML, &m.ExtractedText, &m.Preview, &m.Labels, &m.InReplyTo, &m.References,
		&m.Status, &m.Size, &m.AuthResults, &m.Gatekeeper, &m.Screening, &m.CreatedAt, &m.rawKey, &m.orgID, &m.inboxUUID)
	if err != nil {
		return nil, err
	}
	m.Attachments = []AttachmentInfo{}
	return m, nil
}

func loadAttachments(ctx context.Context, tx pgx.Tx, m *Message) error {
	rows, err := tx.Query(ctx, "select id, filename, content_type, size_bytes from attachments where message_id = $1 order by created_at", m.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a AttachmentInfo
		if err := rows.Scan(&a.ID, &a.Filename, &a.ContentType, &a.Size); err != nil {
			return err
		}
		m.Attachments = append(m.Attachments, a)
	}
	return rows.Err()
}

// ListMessages returns metadata only (no bodies), newest first. Messages held
// by screening are left out; includeReview adds those held for review.
func (s *Service) ListMessages(ctx context.Context, orgID, address string, p Page, label string, includeReview bool) ([]*Message, string, error) {
	cur, err := p.cursor()
	if err != nil {
		return nil, "", err
	}
	var list []*Message
	err = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return err
		}
		q := "select " + messageCols + messageFrom + " where m.inbox_id = $1 and " + viewFor(includeReview).visible()
		args := []any{inbox.ID, p.limit() + 1}
		if label != "" {
			args = append(args, label)
			q += fmt.Sprintf(" and $%d = any(m.labels)", len(args))
		}
		if cur != nil {
			args = append(args, cur.At, cur.ID)
			q += fmt.Sprintf(" and (m.created_at, m.id) < ($%d, $%d)", len(args)-1, len(args))
		}
		rows, err := tx.Query(ctx, q+" order by m.created_at desc, m.id desc limit $2", args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanMessage(rows)
			if err != nil {
				return err
			}
			m.Text, m.HTML, m.ExtractedText = nil, nil, nil
			list = append(list, m)
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

// GetMessage accepts our UUID or the RFC Message-ID. A message held by
// screening is not found unless includeReview is set and it is held for review.
func (s *Service) GetMessage(ctx context.Context, orgID, address, id string, includeReview bool) (*Message, error) {
	var out *Message
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		out, err = s.messageTx(ctx, tx, address, id, viewFor(includeReview))
		return err
	})
	return out, err
}

func (s *Service) messageTx(ctx context.Context, tx pgx.Tx, address, id string, v view) (*Message, error) {
	inbox, err := s.inboxTx(ctx, tx, address)
	if err != nil {
		return nil, err
	}
	m, err := scanMessage(tx.QueryRow(ctx, "select "+messageCols+messageFrom+
		" where m.inbox_id = $1 and (m.id::text = $2 or m.rfc_message_id = $2) and "+v.visible()+
		" order by m.created_at limit 1", inbox.ID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NotFound("message")
	}
	if err != nil {
		return nil, err
	}
	return m, loadAttachments(ctx, tx, m)
}

// RawMessage returns the stored .eml bytes.
func (s *Service) RawMessage(ctx context.Context, orgID, address, id string, includeReview bool) ([]byte, error) {
	m, err := s.GetMessage(ctx, orgID, address, id, includeReview)
	if err != nil {
		return nil, err
	}
	if m.rawKey == nil {
		return nil, NotFound("raw message")
	}
	return s.Blob.Get(ctx, *m.rawKey)
}

func (s *Service) UpdateMessageLabels(ctx context.Context, orgID, address, id string, add, remove []string) (*Message, error) {
	add, remove = uniqueLabels(add), uniqueLabels(remove)
	if err := validateCustomLabels(append(append([]string{}, add...), remove...)); err != nil {
		return nil, err
	}
	var out *Message
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		m, err := s.messageTx(ctx, tx, address, id, viewDefault)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `update messages set labels = array(
			select distinct l from unnest(labels || $2::text[]) l where not (l = any($3::text[]))) where id = $1`,
			m.ID, add, remove)
		if err != nil {
			return err
		}
		out, err = s.messageTx(ctx, tx, address, m.ID, viewDefault)
		return err
	})
	return out, err
}

type AttachmentInput struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"` // base64
}

type SendRequest struct {
	To          []string          `json:"to"`
	Cc          []string          `json:"cc"`
	Bcc         []string          `json:"bcc"`
	Subject     string            `json:"subject"`
	Text        string            `json:"text"`
	HTML        string            `json:"html"`
	Labels      []string          `json:"labels"`
	Attachments []AttachmentInput `json:"attachments"`
}

type ReplyRequest struct {
	Text        string            `json:"text"`
	HTML        string            `json:"html"`
	ReplyAll    bool              `json:"reply_all"`
	Labels      []string          `json:"labels"`
	Attachments []AttachmentInput `json:"attachments"`
}

type SendResult struct {
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
}

const (
	maxRecipients   = 50
	maxRequestBytes = 6 << 20
	freeFooterText  = "\n\n--\nSent via multi.ai.inbox"
	freeFooterHTML  = `<p style="color:#888;font-size:12px">Sent via multi.ai.inbox</p>`
)

func normalizeAddrs(list []string) ([]string, error) {
	out := make([]string, 0, len(list))
	for _, a := range list {
		a = strings.ToLower(strings.TrimSpace(a))
		if !mailx.ValidAddress(a) {
			return nil, Validation("invalid email address %q", a)
		}
		out = append(out, a)
	}
	return out, nil
}

func decodeAttachments(in []AttachmentInput) ([]mailx.Attachment, error) {
	out := make([]mailx.Attachment, 0, len(in))
	total := 0
	for _, a := range in {
		data, err := base64.StdEncoding.DecodeString(a.Content)
		if err != nil {
			return nil, Validation("attachment %q content is not valid base64", a.Filename)
		}
		total += len(data)
		if total > maxRequestBytes {
			return nil, errf(http.StatusRequestEntityTooLarge, "payload_too_large", "attachments exceed 6 MB")
		}
		out = append(out, mailx.Attachment{Filename: a.Filename, ContentType: a.ContentType, Data: data})
	}
	return out, nil
}

type outbound struct {
	to, cc, bcc []string
	subject     string
	text, html  string
	labels      []string
	attachments []mailx.Attachment
	threadID    string // empty starts a new thread
	inReplyTo   string
	references  []string
	unscanned   bool // attachments accepted without a virus scan (fail open)
}

func (s *Service) Send(ctx context.Context, orgID, address string, req SendRequest, idemKey string) (*SendResult, error) {
	to, err := normalizeAddrs(req.To)
	if err != nil {
		return nil, err
	}
	cc, err := normalizeAddrs(req.Cc)
	if err != nil {
		return nil, err
	}
	bcc, err := normalizeAddrs(req.Bcc)
	if err != nil {
		return nil, err
	}
	if len(to) == 0 {
		return nil, Validation("at least one recipient in to is required")
	}
	if req.Text == "" && req.HTML == "" {
		return nil, Validation("text or html is required")
	}
	labels := uniqueLabels(req.Labels)
	if err := validateCustomLabels(labels); err != nil {
		return nil, err
	}
	atts, err := decodeAttachments(req.Attachments)
	if err != nil {
		return nil, err
	}
	unscanned, err := s.scanAttachments(ctx, atts)
	if err != nil {
		return nil, err
	}
	o := outbound{to: to, cc: cc, bcc: bcc, subject: req.Subject, text: req.Text, html: req.HTML, labels: labels, attachments: atts,
		unscanned: unscanned}
	return s.idempotent(ctx, orgID, idemKey, req, func(tx pgx.Tx) (*SendResult, error) {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return nil, err
		}
		return s.queue(ctx, tx, orgID, inbox, o)
	})
}

func (s *Service) Reply(ctx context.Context, orgID, address, id string, req ReplyRequest, idemKey string) (*SendResult, error) {
	if req.Text == "" && req.HTML == "" {
		return nil, Validation("text or html is required")
	}
	labels := uniqueLabels(req.Labels)
	if err := validateCustomLabels(labels); err != nil {
		return nil, err
	}
	atts, err := decodeAttachments(req.Attachments)
	if err != nil {
		return nil, err
	}
	unscanned, err := s.scanAttachments(ctx, atts)
	if err != nil {
		return nil, err
	}
	return s.idempotent(ctx, orgID, idemKey, req, func(tx pgx.Tx) (*SendResult, error) {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err != nil {
			return nil, err
		}
		// Replying to a held message would let its content steer the agent.
		parent, err := s.messageTx(ctx, tx, address, id, viewDefault)
		if err != nil {
			return nil, err
		}
		o := replyOutbound(inbox, parent, req.ReplyAll)
		o.text, o.html, o.labels, o.attachments = req.Text, req.HTML, labels, atts
		o.unscanned = unscanned
		return s.queue(ctx, tx, orgID, inbox, o)
	})
}

// replyOutbound addresses a reply to parent and sets its threading headers.
// Reply all copies the other recipients of parent, never the inbox itself.
func replyOutbound(inbox *Inbox, parent *Message, replyAll bool) outbound {
	target := parent.From
	if parent.Direction == "out" && len(parent.To) > 0 {
		target = parent.To[0]
	}
	o := outbound{to: []string{target}, subject: mailx.ReplySubject(parent.Subject), threadID: parent.ThreadID,
		inReplyTo: parent.MessageID, references: append(append([]string{}, parent.References...), parent.MessageID)}
	if replyAll {
		seen := map[string]bool{inbox.InboxID: true, target: true}
		for _, a := range append(append([]string{}, parent.To...), parent.Cc...) {
			if !seen[a] {
				seen[a] = true
				o.cc = append(o.cc, a)
			}
		}
	}
	return o
}

// queue renders the MIME, stores it and inserts an outbound message for the sender worker.
func (s *Service) queue(ctx context.Context, tx pgx.Tx, orgID string, inbox *Inbox, o outbound) (*SendResult, error) {
	if n := len(o.to) + len(o.cc) + len(o.bcc); n > maxRecipients {
		return nil, Validation("at most %d recipients per message, got %d", maxRecipients, n)
	}
	if err := sendingPaused(ctx, tx, orgID, inbox.ID, inbox.InboxID); err != nil {
		return nil, err
	}
	var planName string
	var sentToday, sentMonth, sentBurst int
	var burstWait float64 // seconds until the oldest send leaves the 5 minute window
	if err := tx.QueryRow(ctx, `select o.plan,
		(select count(*) from messages where org_id = o.id and direction = 'out' and created_at > now() - interval '1 day'),
		(select count(*) from messages where org_id = o.id and direction = 'out' and created_at >= date_trunc('month', now())),
		b.n, coalesce(extract(epoch from b.oldest + interval '5 minutes' - now()), 0)::float8
		from organizations o, lateral (select count(*) as n, min(created_at) as oldest from messages
			where org_id = o.id and direction = 'out' and created_at > now() - interval '5 minutes') b
		where o.id = $1`, orgID).Scan(&planName, &sentToday, &sentMonth, &sentBurst, &burstWait); err != nil {
		return nil, err
	}
	plan := planOf(planName)
	if sentToday >= plan.SendsPerDay {
		return nil, errf(http.StatusTooManyRequests, "limit_exceeded", "daily send limit of %d reached on the %s plan", plan.SendsPerDay, plan.Name)
	}
	if sentMonth >= plan.SendsPerMonth {
		return nil, errf(http.StatusTooManyRequests, "limit_exceeded", "monthly send limit of %d reached on the %s plan", plan.SendsPerMonth, plan.Name)
	}
	if sentBurst >= plan.SendsPer5Min {
		e := errf(http.StatusTooManyRequests, "rate_limited", "send burst limit of %d per 5 minutes reached on the %s plan", plan.SendsPer5Min, plan.Name)
		e.RetryAfter = max(1, int(math.Ceil(burstWait)))
		return nil, e
	}
	if o.unscanned {
		o.labels = uniqueLabels(append(o.labels, "av-unscanned"))
	}
	if plan.Footer {
		o.text += freeFooterText
		if o.html != "" {
			o.html += freeFooterHTML
		}
	}

	rfcID := mailx.NewMessageID(mailx.DomainOf(inbox.InboxID))
	raw, err := mailx.Build(mailx.Outgoing{FromName: inbox.DisplayName, From: inbox.InboxID, To: o.to, Cc: o.cc,
		Subject: o.subject, Text: o.text, HTML: o.html, MessageID: rfcID, InReplyTo: o.inReplyTo,
		References: o.references, Attachments: o.attachments, Date: s.now()})
	if err != nil {
		return nil, err
	}
	now := s.now()
	rawKey := fmt.Sprintf("raw/%s/%s/%s.eml", orgID, now.Format("2006/01"), strings.Trim(rfcID, "<>"))
	if err := s.Blob.Put(ctx, rawKey, raw); err != nil {
		return nil, fmt.Errorf("store raw: %w", err)
	}

	threadID := o.threadID
	preview := mailx.Preview(o.text, 200)
	if threadID == "" {
		if err := tx.QueryRow(ctx, `insert into threads (org_id, inbox_id, subject, preview) values ($1, $2, $3, $4) returning id`,
			orgID, inbox.ID, o.subject, preview).Scan(&threadID); err != nil {
			return nil, err
		}
	}
	var inReplyTo any
	if o.inReplyTo != "" {
		inReplyTo = o.inReplyTo
	}
	var msgID string
	err = tx.QueryRow(ctx, `insert into messages (org_id, inbox_id, thread_id, direction, rfc_message_id, in_reply_to, refs,
		from_addr, to_addrs, cc_addrs, bcc_addrs, subject, text_body, html_body, preview, labels, raw_key, size_bytes,
		status, next_attempt_at)
		values ($1, $2, $3, 'out', $4, $5, $6, $7, $8, $9, $10, $11, $12, nullif($13, ''), $14, $15, $16, $17, 'queued', now())
		returning id`,
		orgID, inbox.ID, threadID, rfcID, inReplyTo, nonNil(o.references), inbox.InboxID, nonNil(o.to), nonNil(o.cc), nonNil(o.bcc), o.subject, o.text,
		o.html, preview, append([]string{"sent"}, o.labels...), rawKey, len(raw)).Scan(&msgID)
	if err != nil {
		return nil, err
	}
	for i, a := range o.attachments {
		key := fmt.Sprintf("att/%s/%s/%d", orgID, msgID, i)
		if err := s.Blob.Put(ctx, key, a.Data); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `insert into attachments (org_id, message_id, filename, content_type, size_bytes, storage_key)
			values ($1, $2, $3, $4, $5, $6)`, orgID, msgID, a.Filename, a.ContentType, len(a.Data), key); err != nil {
			return nil, err
		}
	}
	if err := touchThread(ctx, tx, threadID, o.subject, preview, o.labels); err != nil {
		return nil, err
	}
	if o.unscanned {
		if err := s.alertAVUnscanned(ctx, tx, orgID, inbox.ID, inbox.InboxID, "api"); err != nil {
			return nil, err
		}
	}
	return &SendResult{MessageID: rfcID, ThreadID: threadID}, nil
}

// touchThread counts a visible message in its thread. A thread created by a
// held message has no visible message yet (message_count 0), so the first
// visible one also sets the subject.
func touchThread(ctx context.Context, tx pgx.Tx, threadID, subject, preview string, labels []string) error {
	_, err := tx.Exec(ctx, `update threads set message_count = message_count + 1, last_message_at = now(),
		subject = case when message_count = 0 then $4 else subject end,
		preview = $2, labels = array(select distinct l from unnest(labels || $3::text[]) l) where id = $1`,
		threadID, preview, labels, subject)
	return err
}

// idempotent replays the stored response for a repeated Idempotency-Key within 24 hours.
func (s *Service) idempotent(ctx context.Context, orgID, key string, req any, fn func(pgx.Tx) (*SendResult, error)) (*SendResult, error) {
	if len(key) > 256 {
		return nil, Validation("Idempotency-Key must be at most 256 characters")
	}
	body, _ := json.Marshal(req)
	hash := sha256.Sum256(body)
	var out *SendResult
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		if key != "" {
			if _, err := tx.Exec(ctx, "delete from idempotency_keys where created_at < now() - interval '24 hours'"); err != nil {
				return err
			}
			var storedHash []byte
			var stored json.RawMessage
			err := tx.QueryRow(ctx, "select request_hash, response from idempotency_keys where key = $1 for update", key).Scan(&storedHash, &stored)
			if err == nil {
				if string(storedHash) != string(hash[:]) {
					return errf(http.StatusConflict, "conflict", "Idempotency-Key was used with a different request")
				}
				out = &SendResult{}
				return json.Unmarshal(stored, out)
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		res, err := fn(tx)
		if err != nil {
			return err
		}
		out = res
		if key != "" {
			resp, _ := json.Marshal(res)
			_, err = tx.Exec(ctx, "insert into idempotency_keys (org_id, key, request_hash, response) values ($1, $2, $3, $4)",
				orgID, key, hash[:], resp)
			if isUniqueViolation(err) {
				// A concurrent request with the same key won; this one rolls back.
				return errf(http.StatusConflict, "conflict", "a request with this Idempotency-Key is already in progress; retry to get its result")
			}
		}
		return err
	})
	return out, err
}

// GetAttachment returns attachment metadata and its bytes.
func (s *Service) GetAttachment(ctx context.Context, orgID, address, messageID, attachmentID string, includeReview bool) (*AttachmentInfo, []byte, error) {
	var info *AttachmentInfo
	var key string
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		m, err := s.messageTx(ctx, tx, address, messageID, viewFor(includeReview))
		if err != nil {
			return err
		}
		a := &AttachmentInfo{}
		err = tx.QueryRow(ctx, `select id, filename, content_type, size_bytes, storage_key from attachments
			where message_id = $1 and id::text = $2`, m.ID, attachmentID).Scan(&a.ID, &a.Filename, &a.ContentType, &a.Size, &key)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("attachment")
		}
		info = a
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	data, err := s.Blob.Get(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("load attachment: %w", err)
	}
	return info, data, nil
}
