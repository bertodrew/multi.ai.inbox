package auth

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
)

// captureRelay is a local SMTP server that keeps what it receives: the mailer
// test never opens a connection to a real relay.
type captureRelay struct {
	mu   sync.Mutex
	from string
	to   []string
	raw  []byte
}

func (c *captureRelay) NewSession(*smtp.Conn) (smtp.Session, error) {
	return &captureSession{c: c}, nil
}

type captureSession struct {
	c    *captureRelay
	from string
	to   []string
}

func (s *captureSession) Reset()                                   { s.from, s.to = "", nil }
func (s *captureSession) Logout() error                            { return nil }
func (s *captureSession) Mail(f string, _ *smtp.MailOptions) error { s.from = f; return nil }
func (s *captureSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}
func (s *captureSession) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	s.c.from, s.c.to, s.c.raw = s.from, s.to, raw
	return nil
}

func relay(t *testing.T) (*captureRelay, string) {
	t.Helper()
	cap := &captureRelay{}
	srv := smtp.NewServer(cap)
	srv.Domain = "relay.test"
	srv.AllowInsecureAuth = true
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return cap, l.Addr().String()
}

func TestRelayMailerSendsPlainTextMessage(t *testing.T) {
	cap, addr := relay(t)
	m := &RelayMailer{Relay: "smtp+insecure://" + addr, From: "no-reply@agents.test", Helo: "mai.test", Timeout: 5 * time.Second}
	if err := m.Send(context.Background(), Message{To: "person@example.com", Subject: "Your sign in link", Text: "open\nhttps://x.test/?a=1"}); err != nil {
		t.Fatal(err)
	}
	cap.mu.Lock()
	defer cap.mu.Unlock()
	if cap.from != "no-reply@agents.test" {
		t.Fatalf("envelope from = %q", cap.from)
	}
	if len(cap.to) != 1 || cap.to[0] != "person@example.com" {
		t.Fatalf("envelope to = %v", cap.to)
	}
	raw := string(cap.raw)
	for _, want := range []string{"From: <no-reply@agents.test>", "To: <person@example.com>", "Subject: Your sign in link",
		"Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: base64", "Message-ID: <"} {
		if !strings.Contains(raw, want) {
			t.Errorf("message missing %q\n%s", want, raw)
		}
	}
	head, body, ok := strings.Cut(raw, "\r\n\r\n")
	if !ok {
		t.Fatalf("no header/body split\n%s", raw)
	}
	if strings.Contains(head, "\n\n") {
		t.Error("headers must use CRLF")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", ""))
	if err != nil {
		t.Fatalf("body is not base64: %v", err)
	}
	if !strings.Contains(string(decoded), "https://x.test/?a=1") {
		t.Fatalf("body = %q", decoded)
	}
}

func TestRelayMailerRequiresConfiguration(t *testing.T) {
	m := &RelayMailer{From: "no-reply@agents.test"}
	if err := m.Send(context.Background(), Message{To: "a@b.test"}); err == nil {
		t.Fatal("a mailer without a relay must fail loudly")
	}
	m = &RelayMailer{Relay: "smtp+insecure://127.0.0.1:1"}
	if err := m.Send(context.Background(), Message{To: "a@b.test"}); err == nil {
		t.Fatal("a mailer without a sender must fail loudly")
	}
}

func TestRelayMailerRejectsHeaderInjection(t *testing.T) {
	cap, addr := relay(t)
	m := &RelayMailer{Relay: "smtp+insecure://" + addr, From: "no-reply@agents.test", Timeout: 5 * time.Second}
	for _, msg := range []Message{
		{To: "person@example.com", Subject: "Hello\r\nBcc: victim@example.com", Text: "x"},
		{To: "person@example.com\r\nRcpt To: x@y.test", Subject: "Hello", Text: "x"},
		{To: "not-an-address", Subject: "Hello", Text: "x"},
		{To: "person@example.com", Subject: "Hello", Text: strings.Repeat("x", 10)},
	} {
		err := m.Send(context.Background(), msg)
		if msg.Text == strings.Repeat("x", 10) {
			if err != nil {
				t.Fatalf("a long body must be fine: %v", err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("accepted a message with a line break in a header: %+v", msg)
		}
	}
	_ = cap
}
