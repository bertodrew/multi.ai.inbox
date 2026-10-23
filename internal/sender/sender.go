// Package sender delivers queued messages: agent to agent mail is ingested
// locally, everything else goes through a relay or directly to the recipient MX.
package sender

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/mailx"
)

type Sender struct {
	Svc      *core.Service
	Relay    string // smtp://user:pass@host:587 (STARTTLS) or smtps://user:pass@host:465; empty = direct MX
	HeloName string
	Log      *slog.Logger
	// TLSConfig overrides TLS settings (tests use a self signed relay).
	TLSConfig *tls.Config
}

func (s *Sender) Run(ctx context.Context) {
	for {
		n, err := s.RunOnce(ctx)
		if err != nil {
			s.Log.Error("sender batch failed", "err", err)
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

// RunOnce processes one batch and returns how many jobs it handled.
func (s *Sender) RunOnce(ctx context.Context) (int, error) {
	jobs, err := s.Svc.ClaimOutbound(ctx, 10)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		sendErr := s.deliver(ctx, j)
		if sendErr != nil {
			s.Log.Warn("delivery attempt failed", "message", j.ID, "attempt", j.Attempts, "err", sendErr)
		}
		if err := s.Svc.FinishOutbound(ctx, j, sendErr); err != nil {
			return len(jobs), fmt.Errorf("finish %s: %w", j.ID, err)
		}
	}
	return len(jobs), nil
}

func (s *Sender) deliver(ctx context.Context, j core.OutboundJob) error {
	raw, err := s.Svc.Blob.Get(ctx, j.RawKey)
	if err != nil {
		return fmt.Errorf("load raw: %w", err)
	}
	if j.DKIMKeyPEM != "" {
		if raw, err = sign(raw, j); err != nil {
			return fmt.Errorf("dkim: %w", err)
		}
	}
	var local []*core.LocalRecipient
	var remote []string
	for _, r := range j.Recipients {
		rc, err := s.Svc.ResolveRecipient(ctx, r)
		if err != nil {
			return err
		}
		if rc != nil {
			local = append(local, rc)
		} else {
			remote = append(remote, r)
		}
	}
	if len(local) > 0 {
		meta := core.InboundMeta{MailFrom: j.Address, AuthResults: map[string]any{"dkim": "pass", "spf": "pass", "dmarc": "pass", "internal": true}}
		// Agent to agent mail skips the SMTP edge, so it is scanned here;
		// infected mail fails for every recipient, remote ones included.
		av, err := s.Svc.ScanInternal(ctx, raw)
		if err != nil {
			return err
		}
		if av != "" {
			meta.AuthResults["antivirus"] = av
		}
		if av == "unscanned" {
			meta.Labels = []string{"av-unscanned"}
		}
		if err := s.Svc.Ingest(ctx, raw, local, meta); err != nil {
			return fmt.Errorf("local delivery: %w", err)
		}
	}
	if len(remote) == 0 {
		return nil
	}
	if s.Relay != "" {
		return s.viaRelay(raw, j.Address, remote)
	}
	return s.direct(ctx, raw, j.Address, remote)
}

func sign(raw []byte, j core.OutboundJob) ([]byte, error) {
	block, _ := pem.Decode([]byte(j.DKIMKeyPEM))
	if block == nil {
		return nil, errors.New("invalid DKIM key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err = dkim.Sign(&out, bytes.NewReader(raw), &dkim.SignOptions{
		Domain: j.DKIMDomain, Selector: j.DKIMSel, Signer: key,
		HeaderKeys: []string{"From", "To", "Cc", "Subject", "Date", "Message-Id", "In-Reply-To", "References", "Content-Type", "Mime-Version"},
	})
	return out.Bytes(), err
}

func (s *Sender) viaRelay(raw []byte, from string, rcpts []string) error {
	u, err := url.Parse(s.Relay)
	if err != nil {
		return fmt.Errorf("relay url: %w", err)
	}
	tlsCfg := s.tls(u.Hostname())
	var c *smtp.Client
	switch u.Scheme {
	case "smtps":
		c, err = smtp.DialTLS(u.Host, tlsCfg)
	case "smtp":
		c, err = smtp.DialStartTLS(u.Host, tlsCfg)
	case "smtp+insecure": // local development relays only
		c, err = smtp.Dial(u.Host)
	default:
		return fmt.Errorf("unsupported relay scheme %q", u.Scheme)
	}
	if err != nil {
		return fmt.Errorf("dial relay: %w", err)
	}
	defer c.Close()
	if u.User != nil {
		pass, _ := u.User.Password()
		if err := c.Auth(sasl.NewPlainClient("", u.User.Username(), pass)); err != nil {
			return classify(fmt.Errorf("relay auth: %w", err))
		}
	}
	return transmit(c, raw, from, rcpts)
}

// direct delivers to each recipient domain's MX with opportunistic STARTTLS.
func (s *Sender) direct(ctx context.Context, raw []byte, from string, rcpts []string) error {
	byDomain := map[string][]string{}
	for _, r := range rcpts {
		d := mailx.DomainOf(r)
		byDomain[d] = append(byDomain[d], r)
	}
	for domain, list := range byDomain {
		mxs, err := net.DefaultResolver.LookupMX(ctx, domain)
		if err != nil || len(mxs) == 0 {
			mxs = []*net.MX{{Host: domain, Pref: 0}} // implicit MX (RFC 5321 5.1)
		}
		sort.Slice(mxs, func(a, b int) bool { return mxs[a].Pref < mxs[b].Pref })
		var lastErr error
		for _, mx := range mxs {
			host := strings.TrimSuffix(mx.Host, ".")
			lastErr = s.deliverTo(host, raw, from, list)
			if lastErr == nil || errors.Is(lastErr, core.ErrPermanent) {
				break
			}
		}
		if lastErr != nil {
			return fmt.Errorf("%s: %w", domain, lastErr)
		}
	}
	return nil
}

func (s *Sender) deliverTo(host string, raw []byte, from string, rcpts []string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "25"), 30*time.Second)
	if err != nil {
		return err
	}
	// Opportunistic TLS (RFC 7435): encrypt whenever offered; most MX
	// certificates do not match their hostname. MTA-STS and DANE add
	// authenticated TLS per domain in a later step.
	cfg := s.tls(host)
	if s.TLSConfig == nil {
		cfg.InsecureSkipVerify = true //nolint:gosec // opportunistic SMTP TLS, see comment above
	}
	conn, err = startTLS(conn, s.HeloName, cfg)
	if err != nil {
		return err
	}
	c := smtp.NewClient(conn)
	defer c.Close()
	if err := c.Hello(s.HeloName); err != nil {
		return classify(err)
	}
	return transmit(c, raw, from, rcpts)
}

// startTLS negotiates STARTTLS when the server offers it, using our HELO
// name (go-smtp only exposes STARTTLS with a default "localhost" EHLO). The
// returned connection replays a greeting so the SMTP client can EHLO again,
// as RFC 3207 requires after TLS starts.
func startTLS(conn net.Conn, helo string, cfg *tls.Config) (net.Conn, error) {
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	defer conn.SetDeadline(time.Time{}) //nolint:errcheck // best effort reset
	tp := textproto.NewReader(bufio.NewReader(conn))
	if _, _, err := tp.ReadResponse(220); err != nil {
		conn.Close()
		return nil, classify(err)
	}
	if _, err := fmt.Fprintf(conn, "EHLO %s\r\n", helo); err != nil {
		conn.Close()
		return nil, err
	}
	_, ext, err := tp.ReadResponse(250)
	if err != nil {
		conn.Close()
		return nil, classify(err)
	}
	if !strings.Contains(strings.ToUpper(ext), "STARTTLS") {
		return greeted{Conn: conn, r: io.MultiReader(strings.NewReader("220 resumed\r\n"), conn)}, nil
	}
	if _, err := io.WriteString(conn, "STARTTLS\r\n"); err != nil {
		conn.Close()
		return nil, err
	}
	if _, _, err := tp.ReadResponse(220); err != nil {
		conn.Close()
		return nil, classify(err)
	}
	tlsConn := tls.Client(conn, cfg)
	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("starttls handshake: %w", err)
	}
	return greeted{Conn: tlsConn, r: io.MultiReader(strings.NewReader("220 resumed\r\n"), tlsConn)}, nil
}

type greeted struct {
	net.Conn
	r io.Reader
}

func (g greeted) Read(p []byte) (int, error) { return g.r.Read(p) }

func transmit(c *smtp.Client, raw []byte, from string, rcpts []string) error {
	if err := c.Mail(from, nil); err != nil {
		return classify(err)
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r, nil); err != nil {
			return classify(err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return classify(err)
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return classify(err)
	}
	return c.Quit()
}

func (s *Sender) tls(host string) *tls.Config {
	if s.TLSConfig != nil {
		cfg := s.TLSConfig.Clone()
		cfg.ServerName = host
		return cfg
	}
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
}

// classify wraps 5xx SMTP replies as permanent failures.
func classify(err error) error {
	var se *smtp.SMTPError
	if errors.As(err, &se) && se.Code >= 500 {
		return fmt.Errorf("%w: %v", core.ErrPermanent, err)
	}
	return err
}
