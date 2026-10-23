// Package clamavtest provides a fake clamd for tests: it speaks the INSTREAM
// protocol and reports the EICAR test string as infected.
package clamavtest

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// EICAR is the standard antivirus test string, assembled at run time so the
// source files themselves do not trip scanners.
var EICAR = `X5O!P%@AP[4\PZX54(P^)7CC)7}$` + "EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*"

// Signature is what the fake reports for EICAR.
const Signature = "Win.Test.EICAR_HDB-1"

// Fake starts a fake clamd. It reassembles the INSTREAM chunks and answers
// like clamd; with hang set it reads the stream and never answers. got
// returns the next stream received, or nil after a second.
func Fake(t *testing.T, hang bool) (addr string, got func() []byte) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	received := make(chan []byte, 64)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go serve(conn, hang, received)
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

func serve(conn net.Conn, hang bool, received chan<- []byte) {
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
	select {
	case received <- stream.Bytes():
	default: // nobody reads the streams; never block the fake
	}
	if hang {
		time.Sleep(2 * time.Second)
		return
	}
	reply := "stream: OK\x00"
	if bytes.Contains(stream.Bytes(), []byte(EICAR)) {
		reply = "stream: " + Signature + " FOUND\x00"
	}
	_, _ = conn.Write([]byte(reply))
}

// ClosedAddr returns a local address nothing listens on.
func ClosedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}
