package mailx

import (
	"bufio"
	"bytes"
	"io"
	"net/textproto"
	"strings"

	"github.com/emersion/go-message"
)

// DSN is the useful part of a delivery status notification (RFC 3464).
type DSN struct {
	// OriginalMessageID is the Message-ID of the returned message, with
	// angle brackets, taken from the returned headers part.
	OriginalMessageID string
	// Failed lists recipients with Action: failed and a 5.x.x status.
	Failed []DSNRecipient
}

type DSNRecipient struct {
	Address    string
	Status     string
	Diagnostic string
}

const maxDSNPart = 1 << 20

// ParseDSN returns the report when raw is a multipart/report with
// report-type=delivery-status, or nil for any other message.
func ParseDSN(raw []byte) *DSN {
	e, err := message.Read(bytes.NewReader(raw))
	if e == nil || (err != nil && !message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err)) {
		return nil
	}
	mt, params, err := e.Header.ContentType()
	if err != nil || mt != "multipart/report" || !strings.EqualFold(params["report-type"], "delivery-status") {
		return nil
	}
	mr := e.MultipartReader()
	if mr == nil {
		return nil
	}
	d := &DSN{}
	for {
		p, err := mr.NextPart()
		if err != nil {
			break // io.EOF or a malformed tail: keep what was parsed
		}
		ct, _, _ := p.Header.ContentType()
		body := bufio.NewReader(io.LimitReader(p.Body, maxDSNPart))
		switch ct {
		case "message/delivery-status", "message/global-delivery-status":
			d.Failed = append(d.Failed, failedRecipients(body)...)
		case "message/rfc822", "text/rfc822-headers", "message/rfc822-headers", "message/global", "message/global-headers":
			if h, err := textproto.NewReader(body).ReadMIMEHeader(); err == nil || len(h) > 0 {
				if id := strings.TrimSpace(h.Get("Message-Id")); id != "" {
					d.OriginalMessageID = bracket(strings.Trim(id, "<>"))
				}
			}
		}
	}
	return d
}

// failedRecipients reads the per message block and then one block per
// recipient, each a header style field list separated by blank lines.
func failedRecipients(r *bufio.Reader) []DSNRecipient {
	tp := textproto.NewReader(r)
	var out []DSNRecipient
	for {
		h, err := tp.ReadMIMEHeader()
		if rcpt := h.Get("Final-Recipient"); rcpt != "" &&
			strings.EqualFold(strings.TrimSpace(h.Get("Action")), "failed") &&
			strings.HasPrefix(strings.TrimSpace(h.Get("Status")), "5") {
			// "rfc822; user@example.com"
			if _, addr, ok := strings.Cut(rcpt, ";"); ok {
				rcpt = addr
			}
			out = append(out, DSNRecipient{
				Address:    strings.ToLower(strings.Trim(strings.TrimSpace(rcpt), "<>")),
				Status:     strings.TrimSpace(h.Get("Status")),
				Diagnostic: strings.TrimSpace(h.Get("Diagnostic-Code")),
			})
		}
		if err != nil {
			return out
		}
	}
}
