package mx

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/bertodrew/multi.ai.inbox/internal/clamav"
	"github.com/bertodrew/multi.ai.inbox/internal/clamav/clamavtest"
)

// The SMTP edge rejects infected mail at DATA and, when fail closed, defers
// mail it cannot scan. Both paths end before ingest, so no database is needed.
func TestDataAntivirus(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	addr, _ := clamavtest.Fake(t, false)
	msg := "From: a@ext.test\r\nTo: b@agents.test\r\nSubject: invoice\r\n\r\n" + clamavtest.EICAR + "\r\n"

	s := &session{b: &Backend{Log: log, MaxBytes: 1 << 20, Clamd: &clamav.Clamd{Addr: addr}}, remote: "127.0.0.1:1"}
	var se *smtp.SMTPError
	if err := s.Data(strings.NewReader(msg)); !errors.As(err, &se) || se.Code != 554 || se.EnhancedCode != (smtp.EnhancedCode{5, 7, 1}) {
		t.Fatalf("infected: %v", err)
	}

	s = &session{b: &Backend{Log: log, MaxBytes: 1 << 20, Clamd: &clamav.Clamd{Addr: clamavtest.ClosedAddr(t), Timeout: time.Second}, ClamdFailClosed: true}}
	if err := s.Data(strings.NewReader("Subject: x\r\n\r\nclean\r\n")); !errors.As(err, &se) || se.Code != 451 || se.EnhancedCode != (smtp.EnhancedCode{4, 7, 1}) {
		t.Fatalf("fail closed: %v", err)
	}
}
