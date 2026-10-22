// Package mailx parses inbound MIME into agent friendly fields and builds
// outbound MIME with correct threading headers.
package mailx

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	_ "github.com/emersion/go-message/charset" // decode non UTF-8 charsets
	"github.com/emersion/go-message/mail"
)

const maxPartBytes = 25 << 20

type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

type Parsed struct {
	MessageID   string // with angle brackets, e.g. <abc@example.com>
	InReplyTo   string
	References  []string
	From        string
	To          []string
	Cc          []string
	Subject     string
	Text        string
	HTML        string
	Attachments []Attachment
}

func Parse(raw []byte) (*Parsed, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse mime: %w", err)
	}
	h := mr.Header
	p := &Parsed{}
	if id, err := h.MessageID(); err == nil && id != "" {
		p.MessageID = bracket(id)
	}
	if ids, err := h.MsgIDList("In-Reply-To"); err == nil && len(ids) > 0 {
		p.InReplyTo = bracket(ids[0])
	}
	if ids, err := h.MsgIDList("References"); err == nil {
		for _, id := range ids {
			p.References = append(p.References, bracket(id))
		}
	}
	if from, err := h.AddressList("From"); err == nil && len(from) > 0 {
		p.From = from[0].Address
	}
	p.To = addresses(h, "To")
	p.Cc = addresses(h, "Cc")
	p.Subject, _ = h.Subject()

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read part: %w", err)
		}
		body, err := io.ReadAll(io.LimitReader(part.Body, maxPartBytes))
		if err != nil {
			return nil, fmt.Errorf("read part body: %w", err)
		}
		switch ph := part.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := ph.ContentType()
			switch {
			case ct == "text/plain" && p.Text == "":
				p.Text = string(body)
			case ct == "text/html" && p.HTML == "":
				p.HTML = string(body)
			}
		case *mail.AttachmentHeader:
			name, _ := ph.Filename()
			ct, _, _ := ph.ContentType()
			p.Attachments = append(p.Attachments, Attachment{Filename: name, ContentType: ct, Data: body})
		}
	}
	if p.MessageID == "" {
		p.MessageID = NewMessageID("unknown.invalid")
	}
	return p, nil
}

func addresses(h mail.Header, key string) []string {
	list, err := h.AddressList(key)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, strings.ToLower(a.Address))
	}
	return out
}

func bracket(id string) string {
	return "<" + strings.Trim(id, "<> ") + ">"
}

func unbracket(id string) string { return strings.Trim(id, "<> ") }

// NewMessageID returns a unique RFC 5322 Message-ID for domain.
func NewMessageID(domain string) string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), hex.EncodeToString(buf), domain)
}

var subjectPrefix = regexp.MustCompile(`(?i)^\s*((re|fw|fwd|aw|wg|r|i)\s*(\[\d+\])?\s*:\s*)+`)

// NormalizeSubject strips reply and forward prefixes in common languages.
func NormalizeSubject(s string) string {
	return strings.TrimSpace(subjectPrefix.ReplaceAllString(s, ""))
}

// ReplySubject prefixes "Re: " once.
func ReplySubject(s string) string { return "Re: " + NormalizeSubject(s) }

var spaces = regexp.MustCompile(`\s+`)

// Preview returns the first n characters of text with whitespace collapsed.
func Preview(text string, n int) string {
	t := strings.TrimSpace(spaces.ReplaceAllString(text, " "))
	r := []rune(t)
	if len(r) > n {
		return string(r[:n])
	}
	return t
}

type Outgoing struct {
	FromName    string
	From        string
	To          []string
	Cc          []string
	Subject     string
	Text        string
	HTML        string
	MessageID   string
	InReplyTo   string
	References  []string
	Attachments []Attachment
	Date        time.Time
}

// Build renders an outgoing message as MIME. Bcc is never written to headers.
func Build(o Outgoing) ([]byte, error) {
	var h mail.Header
	h.SetDate(o.Date)
	h.SetAddressList("From", []*mail.Address{{Name: o.FromName, Address: o.From}})
	h.SetAddressList("To", toAddrs(o.To))
	if len(o.Cc) > 0 {
		h.SetAddressList("Cc", toAddrs(o.Cc))
	}
	h.SetSubject(o.Subject)
	h.SetMessageID(unbracket(o.MessageID))
	if o.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{unbracket(o.InReplyTo)})
	}
	if len(o.References) > 0 {
		ids := make([]string, 0, len(o.References))
		for _, r := range o.References {
			ids = append(ids, unbracket(r))
		}
		h.SetMsgIDList("References", ids)
	}

	var buf bytes.Buffer
	mw, err := mail.CreateWriter(&buf, h)
	if err != nil {
		return nil, fmt.Errorf("create mime: %w", err)
	}
	iw, err := mw.CreateInline()
	if err != nil {
		return nil, err
	}
	if err := writeInline(iw, "text/plain", o.Text); err != nil {
		return nil, err
	}
	if o.HTML != "" {
		if err := writeInline(iw, "text/html", o.HTML); err != nil {
			return nil, err
		}
	}
	if err := iw.Close(); err != nil {
		return nil, err
	}
	for _, a := range o.Attachments {
		var ah mail.AttachmentHeader
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		ah.SetContentType(ct, nil)
		ah.SetFilename(a.Filename)
		w, err := mw.CreateAttachment(ah)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(a.Data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeInline(iw *mail.InlineWriter, contentType, body string) error {
	var ph mail.InlineHeader
	ph.SetContentType(contentType, map[string]string{"charset": "utf-8"})
	w, err := iw.CreatePart(ph)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, body); err != nil {
		return err
	}
	return w.Close()
}

func toAddrs(list []string) []*mail.Address {
	out := make([]*mail.Address, 0, len(list))
	for _, a := range list {
		out = append(out, &mail.Address{Address: a})
	}
	return out
}

// DomainOf returns the lowercase domain of an address.
func DomainOf(addr string) string {
	at := strings.LastIndexByte(addr, '@')
	if at < 0 {
		return ""
	}
	return strings.ToLower(addr[at+1:])
}

// ValidAddress reports whether s parses as a single bare email address.
func ValidAddress(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && strings.Contains(s, "@")
}
