// Package clamav is a client for the ClamAV daemon. It is shared by every
// path that accepts content: the SMTP edge, agent to agent delivery and
// attachments uploaded through the API.
package clamav

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Clamd scans messages with a ClamAV daemon over TCP using the INSTREAM
// command. clamd runs as its own service (GPL, reached over the network,
// never linked into the product).
type Clamd struct {
	Addr      string        // host:port, e.g. clamd:3310
	Timeout   time.Duration // whole scan, connect included; 0 = 30s
	ChunkSize int           // bytes per INSTREAM chunk; 0 = 64 KiB
}

// Result is a clamd verdict: Infected with the signature name, or clean.
type Result struct {
	Infected  bool
	Signature string
}

// Scan streams data to clamd. A returned error means the message was not
// scanned (clamd unreachable, timed out, or reported an error such as the
// INSTREAM size limit); the caller decides to fail open or closed.
func (c *Clamd) Scan(ctx context.Context, data []byte) (Result, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	chunk := c.ChunkSize
	if chunk <= 0 {
		chunk = 64 << 10
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return Result{}, fmt.Errorf("clamd dial: %w", err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	// Close the connection when the context ends early (cancelled caller).
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	w := bufio.NewWriterSize(conn, chunk+4)
	if _, err := w.WriteString("zINSTREAM\x00"); err != nil {
		return Result{}, fmt.Errorf("clamd write: %w", err)
	}
	var size [4]byte
	for off := 0; off < len(data); off += chunk {
		part := data[off:min(off+chunk, len(data))]
		binary.BigEndian.PutUint32(size[:], uint32(len(part))) //nolint:gosec // chunk is at most 64 KiB
		if _, err := w.Write(size[:]); err != nil {
			return Result{}, fmt.Errorf("clamd write: %w", err)
		}
		if _, err := w.Write(part); err != nil {
			return Result{}, fmt.Errorf("clamd write: %w", err)
		}
	}
	binary.BigEndian.PutUint32(size[:], 0)
	if _, err := w.Write(size[:]); err != nil {
		return Result{}, fmt.Errorf("clamd write: %w", err)
	}
	if err := w.Flush(); err != nil {
		// clamd closes the stream early when the size limit is hit; its
		// reply, if any, is still worth reading.
		if reply, rerr := readReply(conn); rerr == nil && reply != "" {
			return parseReply(reply)
		}
		return Result{}, fmt.Errorf("clamd write: %w", err)
	}
	reply, err := readReply(conn)
	if err != nil {
		return Result{}, fmt.Errorf("clamd read: %w", err)
	}
	return parseReply(reply)
}

// readReply reads one NUL terminated reply (the z prefix asks for NUL).
func readReply(conn net.Conn) (string, error) {
	r := bufio.NewReader(conn)
	line, err := r.ReadString(0)
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\x00\r\n"), nil
}

// parseReply understands "stream: OK", "stream: <signature> FOUND" and
// "<message> ERROR".
func parseReply(reply string) (Result, error) {
	body := strings.TrimSpace(reply)
	if i := strings.Index(body, ": "); i >= 0 && !strings.Contains(body[:i], " ") {
		body = body[i+2:] // drop "stream" (or a session id prefix "1: stream")
	}
	body = strings.TrimPrefix(body, "stream: ")
	switch {
	case body == "OK":
		return Result{}, nil
	case strings.HasSuffix(body, " FOUND"):
		return Result{Infected: true, Signature: strings.TrimSpace(strings.TrimSuffix(body, " FOUND"))}, nil
	case strings.HasSuffix(body, " ERROR"):
		return Result{}, errors.New("clamd: " + strings.TrimSuffix(body, " ERROR"))
	}
	return Result{}, fmt.Errorf("clamd: unexpected reply %q", reply)
}
