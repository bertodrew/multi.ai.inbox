package mailx

import (
	"bufio"
	"bytes"
	"io"
	"net/textproto"
	"strings"

	"github.com/emersion/go-message"
)

// ARF is the useful part of an Abuse Reporting Format feedback report (RFC
// 5965), the format mailbox providers use for feedback loop complaints.
type ARF struct {
	// FeedbackType is the lower case Feedback-Type: abuse, fraud, virus,
	// other, not-spam or auth-failure (RFC 6591).
	FeedbackType string
	// OriginalMessageID is the Message-ID of the reported message, with
	// angle brackets: an Original-Message-ID field when the reporter adds
	// one, else the Message-ID of the embedded message or headers.
	OriginalMessageID string
	// OriginalRcptTo lists the Original-Rcpt-To fields, lower case. Many
	// reporters redact them.
	OriginalRcptTo []string
}

// Complaint reports whether the report is a recipient complaining about the
// message (and not, for example, a not-spam or authentication failure report).
func (a *ARF) Complaint() bool {
	switch a.FeedbackType {
	case "abuse", "fraud", "virus":
		return true
	}
	return false
}

// ParseARF returns the report when raw is a multipart/report with
// report-type=feedback-report, or nil for any other message.
func ParseARF(raw []byte) *ARF {
	e, err := message.Read(bytes.NewReader(raw))
	if e == nil || (err != nil && !message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err)) {
		return nil
	}
	mt, params, err := e.Header.ContentType()
	if err != nil || mt != "multipart/report" || !strings.EqualFold(params["report-type"], "feedback-report") {
		return nil
	}
	mr := e.MultipartReader()
	if mr == nil {
		return nil
	}
	a := &ARF{}
	embeddedID := ""
	for {
		p, err := mr.NextPart()
		if err != nil {
			break // io.EOF or a malformed tail: keep what was parsed
		}
		ct, _, _ := p.Header.ContentType()
		body := bufio.NewReader(io.LimitReader(p.Body, maxDSNPart))
		switch ct {
		case "message/feedback-report":
			h, err := textproto.NewReader(body).ReadMIMEHeader()
			if err != nil && len(h) == 0 {
				continue
			}
			a.FeedbackType = strings.ToLower(strings.TrimSpace(h.Get("Feedback-Type")))
			if id := strings.TrimSpace(h.Get("Original-Message-Id")); id != "" {
				a.OriginalMessageID = bracket(id)
			}
			for _, r := range h.Values("Original-Rcpt-To") {
				if r = strings.ToLower(strings.Trim(strings.TrimSpace(r), "<>")); r != "" {
					a.OriginalRcptTo = append(a.OriginalRcptTo, r)
				}
			}
		case "message/rfc822", "text/rfc822-headers", "message/rfc822-headers", "message/global", "message/global-headers":
			if h, err := textproto.NewReader(body).ReadMIMEHeader(); err == nil || len(h) > 0 {
				if id := strings.TrimSpace(h.Get("Message-Id")); id != "" {
					embeddedID = bracket(id)
				}
			}
		}
	}
	if a.FeedbackType == "" {
		return nil // no machine readable part: not a feedback report
	}
	if a.OriginalMessageID == "" {
		a.OriginalMessageID = embeddedID
	}
	return a
}
