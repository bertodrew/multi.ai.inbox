package mx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
)

// eicar is the standard antivirus test string, assembled at run time so this
// source file itself does not trip scanners.
var eicar = `X5O!P%@AP[4\PZX54(P^)7CC)7}$` + "EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*"

// fakeClamd speaks INSTREAM: it reassembles the chunks and answers like
// clamd. With hang set it reads the stream and never answers.
func fakeClamd(t *testing.T, hang bool) (addr string, got func() []byte) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	received := make(chan []byte, 8)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				r := bufio.NewReader(conn)
				cmd, err := r.ReadString(0)
				if err != nil || cmd != "zINSTREAM\x00" {
					_, _ = conn.Write([]byte("UNKNOWN COMMAND\x00"))
					return
				}
				var stream bytes.Buffer
				for {
					var size [4]byte
					if _, err := io.ReadFull(r, size[:]); err != nil {
						return
					}
					n := binary.BigEndian.Uint32(size[:])
					if n == 0 {
						break
					}
					if _, err := io.CopyN(&stream, r, int64(n)); err != nil {
						return
					}
				}
				received <- stream.Bytes()
				if hang {
					time.Sleep(2 * time.Second)
					return
				}
				reply := "stream: OK\x00"
				if bytes.Contains(stream.Bytes(), []byte(eicar)) {
					reply = "stream: Win.Test.EICAR_HDB-1 FOUND\x00"
				}
				_, _ = conn.Write([]byte(reply))
			}(conn)
		}
	}()
	return l.Addr().String(), func() []byte {
		select {
		case b := <-received:
			return b
		case <-time.After(time.Second):
			return nil
		}
	}
}

func TestClamdScan(t *testing.T) {
	addr, got := fakeClamd(t, false)
	c := &Clamd{Addr: addr, ChunkSize: 7, Timeout: 2 * time.Second} // tiny chunks exercise the framing

	clean := []byte(strings.Repeat("Subject: hello\r\n\r\nquarterly report\r\n", 50))
	res, err := c.Scan(context.Background(), clean)
	if err != nil || res.Infected {
		t.Fatalf("clean: %+v %v", res, err)
	}
	if b := got(); !bytes.Equal(b, clean) {
		t.Fatalf("clamd received %d bytes, want %d", len(b), len(clean))
	}

	res, err = c.Scan(context.Background(), []byte("Subject: x\r\n\r\n"+eicar+"\r\n"))
	if err != nil || !res.Infected || res.Signature != "Win.Test.EICAR_HDB-1" {
		t.Fatalf("eicar: %+v %v", res, err)
	}
}

func TestClamdTimeout(t *testing.T) {
	addr, _ := fakeClamd(t, true)
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
	c := &Clamd{Addr: closedAddr(t), Timeout: time.Second}
	if _, err := c.Scan(context.Background(), []byte("x")); err == nil {
		t.Fatal("unreachable clamd must return an error")
	}
}

func TestParseReply(t *testing.T) {
	for reply, want := range map[string]ScanResult{
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

func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// The SMTP edge rejects infected mail at DATA and, when fail closed, defers
// mail it cannot scan. Both paths end before ingest, so no database is needed.
func TestDataAntivirus(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	addr, _ := fakeClamd(t, false)
	msg := "From: a@ext.test\r\nTo: b@agents.test\r\nSubject: invoice\r\n\r\n" + eicar + "\r\n"

	s := &session{b: &Backend{Log: log, MaxBytes: 1 << 20, Clamd: &Clamd{Addr: addr}}, remote: "127.0.0.1:1"}
	var se *smtp.SMTPError
	if err := s.Data(strings.NewReader(msg)); !errors.As(err, &se) || se.Code != 554 || se.EnhancedCode != (smtp.EnhancedCode{5, 7, 1}) {
		t.Fatalf("infected: %v", err)
	}

	s = &session{b: &Backend{Log: log, MaxBytes: 1 << 20, Clamd: &Clamd{Addr: closedAddr(t), Timeout: time.Second}, ClamdFailClosed: true}}
	if err := s.Data(strings.NewReader("Subject: x\r\n\r\nclean\r\n")); !errors.As(err, &se) || se.Code != 451 || se.EnhancedCode != (smtp.EnhancedCode{4, 7, 1}) {
		t.Fatalf("fail closed: %v", err)
	}
}
