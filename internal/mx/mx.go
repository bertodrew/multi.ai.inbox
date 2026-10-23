// Package mx is the inbound SMTP edge. Unknown recipients are rejected at
// RCPT time (no backscatter); accepted mail is checked, then ingested.
package mx

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/bertodrew/multi.ai.inbox/internal/clamav"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/mailauth"
	"github.com/bertodrew/multi.ai.inbox/internal/mailx"
	"github.com/bertodrew/multi.ai.inbox/internal/metrics"
)

type Backend struct {
	Svc       *core.Service
	Log       *slog.Logger
	MaxBytes  int64
	RspamdURL string // optional, e.g. http://rspamd:11333
	HTTP      *http.Client
	// Clamd scans every message when set (MAI_CLAMD_ADDR). Infected mail is
	// rejected at DATA. When clamd cannot scan, mail is accepted with the
	// av-unscanned label, or refused with a temporary failure when
	// ClamdFailClosed is set (MAI_CLAMD_FAIL_CLOSED=true).
	Clamd           *clamav.Clamd
	ClamdFailClosed bool
	// FBLAddress is an optional shared address for feedback loop reports
	// (MAI_FBL_ADDRESS, e.g. fbl@agents.example.eu), registered with mailbox
	// providers. Complaints sent there are recorded against the original
	// message in whatever organization sent it; the reports are not stored.
	FBLAddress string
	// MailAuth evaluates SPF and DMARC when set (cmd/mai always sets it).
	// DKIM is verified either way.
	MailAuth *MailAuth
	// DNS resolves SPF, DKIM and DMARC records; nil uses net.DefaultResolver.
	DNS mailauth.Resolver
}

// MailAuth configures SPF and DMARC evaluation. Mail that fails DMARC gets
// the dmarc-fail label; under p=quarantine or p=reject it is also labelled
// spam. With Enforce (MAI_DMARC_ENFORCE=true) a failure under p=reject is
// refused with 550 instead.
type MailAuth struct {
	Enforce bool
	Sample  func() int // pct= sampling, tests only
}

func (b *Backend) resolver() mailauth.Resolver {
	if b.DNS != nil {
		return b.DNS
	}
	return net.DefaultResolver
}

// NewServer builds the SMTP edge. With tlsConfig set, STARTTLS is offered.
func NewServer(b *Backend, addr, domain string, tlsConfig *tls.Config) *smtp.Server {
	s := smtp.NewServer(b)
	s.TLSConfig = tlsConfig
	s.Addr = addr
	s.Domain = domain
	s.MaxMessageBytes = b.MaxBytes
	s.MaxRecipients = 50
	s.ReadTimeout = 2 * time.Minute
	s.WriteTimeout = 2 * time.Minute
	return s
}

func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	metrics.SMTPSessions.Inc()
	return &session{b: b, remote: c.Conn().RemoteAddr().String(), helo: c.Hostname()}, nil
}

type session struct {
	b      *Backend
	remote string
	helo   string
	from   string
	rcpts  []*core.LocalRecipient
	fbl    bool // the shared feedback loop address is a recipient
}

func (s *session) Reset()        { s.from, s.rcpts, s.fbl = "", nil, false }
func (s *session) Logout() error { return nil }

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	if s.b.FBLAddress != "" && strings.EqualFold(strings.TrimSpace(to), s.b.FBLAddress) {
		s.fbl = true
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rc, err := s.b.Svc.ResolveRecipient(ctx, to)
	if err != nil {
		s.b.Log.Error("recipient lookup failed", "rcpt", to, "err", err)
		metrics.SMTPRejected.With("recipient_lookup_error").Inc()
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary lookup failure"}
	}
	if rc == nil {
		metrics.SMTPRejected.With("unknown_recipient").Inc()
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
		metrics.SMTPRejected.With("too_large").Inc()
		return &smtp.SMTPError{Code: 552, EnhancedCode: smtp.EnhancedCode{5, 3, 4}, Message: "Message too large"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	auth, labels, err := s.authenticate(ctx, raw)
	if err != nil {
		return err
	}
	antivirus := ""
	if s.b.Clamd != nil {
		res, err := s.b.Clamd.Scan(ctx, raw)
		switch {
		case err != nil && s.b.ClamdFailClosed:
			s.b.Log.Error("clamd unavailable, deferring message", "err", err, "remote", s.remote)
			metrics.SMTPRejected.With("antivirus_unavailable").Inc()
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 7, 1}, Message: "Virus scanner unavailable, try again later"}
		case err != nil:
			s.b.Svc.NoteAVUnscanned("mx", err)
			labels = append(labels, "av-unscanned")
			antivirus = "unscanned"
		case res.Infected:
			s.b.Log.Warn("virus rejected", "signature", res.Signature, "remote", s.remote, "from", s.from)
			metrics.SMTPRejected.With("virus").Inc()
			return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "Message rejected: virus detected (" + res.Signature + ")"}
		default:
			antivirus = "clean"
		}
	}
	if s.fbl {
		if ok, err := s.b.Svc.RecordFeedbackReport(ctx, raw); err != nil {
			s.b.Log.Error("feedback report failed", "err", err, "remote", s.remote)
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary failure, try again"}
		} else if ok {
			s.b.Log.Info("complaint recorded from feedback loop", "remote", s.remote)
		}
		if len(s.rcpts) == 0 {
			return nil
		}
	}
	if antivirus != "" {
		auth["antivirus"] = antivirus
	}
	if s.b.RspamdURL != "" {
		verdict, err := s.rspamd(ctx, raw)
		if err != nil {
			s.b.Log.Warn("rspamd unavailable, accepting without spam score", "err", err)
		} else {
			auth["spam_score"] = verdict.Score
			auth["spam_action"] = verdict.Action
			switch verdict.Action {
			case "reject":
				metrics.SMTPRejected.With("spam").Inc()
				return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "Message rejected as spam"}
			case "add header", "rewrite subject", "soft reject":
				labels = append(labels, "spam")
			}
		}
	}
	if err := s.b.Svc.Ingest(ctx, raw, s.rcpts, core.InboundMeta{MailFrom: s.from, AuthResults: auth, Labels: labels}); err != nil {
		s.b.Log.Error("ingest failed", "err", err, "remote", s.remote)
		metrics.SMTPRejected.With("ingest_error").Inc()
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary failure, try again"}
	}
	metrics.SMTPAccepted.Inc()
	return nil
}

// authenticate verifies DKIM and, with MailAuth set, SPF and DMARC. It
// returns the authentication results to store, labels for the message, and
// an SMTP error when DMARC enforcement refuses the message.
func (s *session) authenticate(ctx context.Context, raw []byte) (map[string]any, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dns := s.b.resolver()
	fromDomain := mailx.DomainOf(headerFrom(raw))
	passed, signed := mailauth.VerifyDKIM(ctx, dns, raw)
	auth := map[string]any{"dkim": dkimStatus(passed, signed, fromDomain)}
	if s.b.MailAuth == nil {
		return auth, nil, nil
	}
	var ip net.IP
	if host, _, err := net.SplitHostPort(s.remote); err == nil {
		ip = net.ParseIP(host)
	}
	spf := mailauth.CheckSPF(ctx, dns, ip, s.from, s.helo)
	spfDomain := mailx.DomainOf(s.from)
	if s.from == "" {
		spfDomain = s.helo
	}
	v := mailauth.CheckDMARC(ctx, dns, mailauth.DMARCInput{FromDomain: fromDomain, DKIMPass: passed, SPF: spf, SPFDomain: spfDomain}, s.b.MailAuth.Sample)
	auth["spf"] = string(spf)
	auth["dmarc"] = v.Result
	if v.Policy != "" {
		auth["dmarc_policy"] = v.Policy
	}
	if v.Result != mailauth.DMARCFail {
		return auth, nil, nil
	}
	labels := []string{"dmarc-fail"}
	switch v.Policy {
	case "reject":
		if s.b.MailAuth.Enforce {
			s.b.Log.Warn("dmarc reject", "from_domain", fromDomain, "remote", s.remote, "spf", spf)
			return nil, nil, &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1},
				Message: "Message rejected: DMARC policy of " + fromDomain + " (p=reject)"}
		}
		labels = append(labels, "spam")
	case "quarantine":
		labels = append(labels, "spam")
	}
	return auth, labels, nil
}

// dkimStatus returns pass when a valid signature aligns with the From domain.
func dkimStatus(passed []string, signed bool, fromDomain string) string {
	if !signed {
		return "none"
	}
	for _, d := range passed {
		d = strings.ToLower(d)
		if d == fromDomain || isSubdomain(fromDomain, d) {
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
