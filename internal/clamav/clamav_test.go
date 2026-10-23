package clamav

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bertodrew/multi.ai.inbox/internal/clamav/clamavtest"
)

func TestClamdScan(t *testing.T) {
	addr, got := clamavtest.Fake(t, false)
	c := &Clamd{Addr: addr, ChunkSize: 7, Timeout: 2 * time.Second} // tiny chunks exercise the framing

	clean := []byte(strings.Repeat("Subject: hello\r\n\r\nquarterly report\r\n", 50))
	res, err := c.Scan(context.Background(), clean)
	if err != nil || res.Infected {
		t.Fatalf("clean: %+v %v", res, err)
	}
	if b := got(); !bytes.Equal(b, clean) {
		t.Fatalf("clamd received %d bytes, want %d", len(b), len(clean))
	}

	res, err = c.Scan(context.Background(), []byte("Subject: x\r\n\r\n"+clamavtest.EICAR+"\r\n"))
	if err != nil || !res.Infected || res.Signature != clamavtest.Signature {
		t.Fatalf("eicar: %+v %v", res, err)
	}
}

func TestClamdTimeout(t *testing.T) {
	addr, _ := clamavtest.Fake(t, true)
	c := &Clamd{Addr: addr, Timeout: 200 * time.Millisecond}
	start := time.Now()
	_, err := c.Scan(context.Background(), []byte("Subject: x\r\n\r\nbody"))
	if err == nil {
		t.Fatal("hanging clamd must time out")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout took %v", time.Since(start))
	}
}

func TestClamdUnreachable(t *testing.T) {
	c := &Clamd{Addr: clamavtest.ClosedAddr(t), Timeout: time.Second}
	if _, err := c.Scan(context.Background(), []byte("x")); err == nil {
		t.Fatal("unreachable clamd must return an error")
	}
}

func TestParseReply(t *testing.T) {
	for reply, want := range map[string]Result{
		"stream: OK":                          {},
		"1: stream: OK":                       {},
		"stream: Eicar-Test-Signature FOUND":  {Infected: true, Signature: "Eicar-Test-Signature"},
		"stream: Doc.Macro.Agent-1 FOUND\r\n": {Infected: true, Signature: "Doc.Macro.Agent-1"},
	} {
		got, err := parseReply(reply)
		if err != nil || got != want {
			t.Errorf("parseReply(%q) = %+v, %v; want %+v", reply, got, err, want)
		}
	}
	for _, reply := range []string{"INSTREAM size limit exceeded. ERROR", "garbage"} {
		if _, err := parseReply(reply); err == nil {
			t.Errorf("parseReply(%q) must fail", reply)
		}
	}
}
