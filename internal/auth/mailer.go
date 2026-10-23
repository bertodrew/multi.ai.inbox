package auth

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// Message is a plain text transactional email. Links are single use and short
// lived, so the body never needs HTML.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Mailer sends a Message. The interface exists so tests never open a socket.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// RelayMailer sends through the same SMTP relay the platform already uses for
// outbound mail (MAI_SMTP_RELAY): smtp:// starts TLS, smtps:// is implicit TLS
// and smtp+insecure:// is for a local development relay only.
type RelayMailer struct {
	Relay     string // MAI_SMTP_RELAY
	From      string // envelope and header sender, e.g. no-reply@agents.example
	Helo      string // the platform hostname
	TLSConfig *tls.Config
	Timeout   time.Duration
}

func (m *RelayMailer) dial(ctx context.Context, u *url.URL) (*smtp.Client, error) {
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	d := &net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("dial relay: %w", err)
	}
	cfg := m.tlsConfig(u.Hostname())
	var c *smtp.Client
	switch u.Scheme {
	case "smtps":
		tc := tls.Client(conn, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("tls relay: %w", err)
		}
		c = smtp.NewClient(tc)
	case "smtp":
		c, err = smtp.NewClientStartTLS(conn, cfg)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("starttls relay: %w", err)
		}
	case "smtp+insecure": // local development relays only
		c = smtp.NewClient(conn)
	default:
		_ = conn.Close()
		return nil, fmt.Errorf("unsupported relay scheme %q", u.Scheme)
	}
	if m.Helo != "" {
		if err := c.Hello(m.Helo); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("relay helo: %w", err)
		}
	}
	if u.User != nil {
		pass, _ := u.User.Password()
		if err := c.Auth(sasl.NewPlainClient("", u.User.Username(), pass)); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("relay auth: %w", err)
		}
	}
	return c, nil
}

func (m *RelayMailer) tlsConfig(host string) *tls.Config {
	if m.TLSConfig != nil {
		cfg := m.TLSConfig.Clone()
		cfg.ServerName = host
		return cfg
	}
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
}

// Send delivers one message. A missing relay or sender is a configuration
// error, never a silent drop: the caller reports it and the user retries.
func (m *RelayMailer) Send(ctx context.Context, msg Message) error {
	if m.Relay == "" {
		return errors.New("auth: MAI_SMTP_RELAY is not set")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := m.compose(msg)
	if err != nil {
		return err
	}
	u, err := url.Parse(m.Relay)
	if err != nil {
		return fmt.Errorf("relay url: %w", err)
	}
	c, err := m.dial(ctx, u)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Mail(m.From, nil); err != nil {
		return fmt.Errorf("relay mail from: %w", err)
	}
	if err := c.Rcpt(msg.To, nil); err != nil {
		return fmt.Errorf("relay rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("relay data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("relay write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("relay close: %w", err)
	}
	return c.Quit()
}

// compose builds an RFC 5322 message with a base64 body, so the relay never
// sees a raw 8 bit or over long line.
func (m *RelayMailer) compose(msg Message) ([]byte, error) {
	from := m.From
	if from == "" {
		return nil, errors.New("auth: mail sender is not set")
	}
	for _, v := range []string{from, msg.To, msg.Subject, m.Helo} {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("auth: header value contains a line break")
		}
	}
	to, err := mail.ParseAddress(msg.To)
	if err != nil {
		return nil, fmt.Errorf("auth: invalid recipient: %w", err)
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return nil, fmt.Errorf("auth: invalid sender: %w", err)
	}
	domain := "localhost"
	if at := strings.LastIndex(from, "@"); at >= 0 {
		domain = from[at+1:]
	}
	id, err := NewToken(16)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: <%s>\r\n", from)
	fmt.Fprintf(&b, "To: <%s>\r\n", to.Address)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", msg.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", id, domain)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("\r\n")
	body := base64.StdEncoding.EncodeToString([]byte(msg.Text))
	for len(body) > 76 {
		b.WriteString(body[:76])
		b.WriteString("\r\n")
		body = body[76:]
	}
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String()), nil
}
