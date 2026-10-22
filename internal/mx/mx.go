// Package mx is the inbound SMTP edge. Unknown recipients are rejected at
// RCPT time (no backscatter); accepted mail is checked, then ingested.
package mx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-smtp"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/mailx"
)

type Backend struct {
	Svc       *core.Service
	Log       *slog.Logger
	MaxBytes  int64
	RspamdURL string // optional, e.g. http://rspamd:11333
	HTTP      *http.Client
}

func NewServer(b *Backend, addr, domain string) *smtp.Server {
	s := smtp.NewServer(b)
	s.Addr = addr
	s.Domain = domain
	s.MaxMessageBytes = b.MaxBytes
	s.MaxRecipients = 50
	s.ReadTimeout = 2 * time.Minute
	s.WriteTimeout = 2 * time.Minute
	return s
}

func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &session{b: b, remote: c.Conn().RemoteAddr().String(), helo: c.Hostname()}, nil
}

type session struct {
	b      *Backend
	remote string
	helo   string
	from   string
	rcpts  []*core.LocalRecipient
}

func (s *session) Reset()        { s.from, s.rcpts = "", nil }
func (s *session) Logout() error { return nil }

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rc, err := s.b.Svc.ResolveRecipient(ctx, to)
	if err != nil {
		s.b.Log.Error("recipient lookup failed", "rcpt", to, "err", err)
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary lookup failure"}
	}
	if rc == nil {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "No such inbox"}
	}
	s.rcpts = append(s.rcpts, rc)
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(r, s.b.MaxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > s.b.MaxBytes {
		return &smtp.SMTPError{Code: 552, EnhancedCode: smtp.EnhancedCode{5, 3, 4}, Message: "Message too large"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	auth := map[string]any{"dkim": verifyDKIM(raw, mailx.DomainOf(headerFrom(raw)))}
	var labels []string
	if s.b.RspamdURL != "" {
		verdict, err := s.rspamd(ctx, raw)
		if err != nil {
			s.b.Log.Warn("rspamd unavailable, accepting without spam score", "err", err)
		} else {
			auth["spam_score"] = verdict.Score
			auth["spam_action"] = verdict.Action
			switch verdict.Action {
			case "reject":
				return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "Message rejected as spam"}
			case "add header", "rewrite subject", "soft reject":
				labels = append(labels, "spam")
			}
		}
	}
	if err := s.b.Svc.Ingest(ctx, raw, s.rcpts, core.InboundMeta{MailFrom: s.from, AuthResults: auth, Labels: labels}); err != nil {
		s.b.Log.Error("ingest failed", "err", err, "remote", s.remote)
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary failure, try again"}
	}
	return nil
}

// verifyDKIM returns pass when a valid signature aligns with the From domain.
func verifyDKIM(raw []byte, fromDomain string) string {
	results, err := dkim.Verify(bytes.NewReader(raw))
	if err != nil || len(results) == 0 {
		return "none"
	}
	for _, v := range results {
		if v.Err == nil && (v.Domain == fromDomain || isSubdomain(fromDomain, v.Domain)) {
			return "pass"
		}
	}
	return "fail"
}

func isSubdomain(child, parent string) bool {
	return len(child) > len(parent) && child[len(child)-len(parent)-1:] == "."+parent
}

func headerFrom(raw []byte) string {
	p, err := mailx.Parse(raw)
	if err != nil {
		return ""
	}
	return p.From
}

type rspamdVerdict struct {
	Score  float64 `json:"score"`
	Action string  `json:"action"`
}

// rspamd asks a local rspamd (C/Lua, kept from the old stack) for a verdict.
func (s *session) rspamd(ctx context.Context, raw []byte) (*rspamdVerdict, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.b.RspamdURL+"/checkv2", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("From", s.from)
	req.Header.Set("Helo", s.helo)
	if host, _, err := net.SplitHostPort(s.remote); err == nil {
		req.Header.Set("Ip", host)
	}
	client := s.b.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rspamd returned %d", resp.StatusCode)
	}
	var v rspamdVerdict
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}
