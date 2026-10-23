package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/dispatcher"
)

// StreamSettings tune GET /v0/events/stream. Zero values use the defaults.
type StreamSettings struct {
	PerKey    int           // open streams per API key, default 5
	Poll      time.Duration // outbox poll interval, default 1s
	Heartbeat time.Duration // comment line that keeps proxies from closing an idle stream, default 15s
	Recheck   time.Duration // how often the API key is checked again (revoked, expired), default 1m
}

func (c StreamSettings) withDefaults() StreamSettings {
	if c.PerKey <= 0 {
		c.PerKey = 5
	}
	if c.Poll <= 0 {
		c.Poll = time.Second
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = 15 * time.Second
	}
	if c.Recheck <= 0 {
		c.Recheck = time.Minute
	}
	return c
}

// streamCounter counts open streams per API key in this process.
type streamCounter struct {
	mu   sync.Mutex
	open map[string]int
}

func (c *streamCounter) acquire(key string, limit int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.open == nil {
		c.open = map[string]int{}
	}
	if c.open[key] >= limit {
		return false
	}
	c.open[key]++
	return true
}

func (c *streamCounter) release(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.open[key]--; c.open[key] <= 0 {
		delete(c.open, key)
	}
}

const streamBatch = 100

// streamEvents serves the organization's events as Server Sent Events. Each
// event carries the webhook envelope as data, its type as event and its
// outbox position as id, so a client that reconnects with Last-Event-ID
// continues where it stopped. Payloads are the webhook payloads: events
// about messages held by screening have their bodies redacted.
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := PrincipalFrom(ctx)
	cfg := s.Streams.withDefaults()
	q := r.URL.Query()
	var types []string
	for _, v := range q["types"] {
		types = append(types, strings.Split(v, ",")...)
	}
	filter, err := s.Svc.NewEventFilter(ctx, p.Org.ID, types, q.Get("inbox"))
	if err != nil {
		s.fail(w, err)
		return
	}
	var cursor *core.EventCursor
	last := r.Header.Get("Last-Event-ID")
	if last == "" {
		last = q.Get("last_event_id") // for clients that cannot set headers
	}
	if last != "" {
		after, err := strconv.ParseInt(last, 10, 64)
		if err != nil || after < 0 {
			s.fail(w, core.Validation("Last-Event-ID must be an event id from this stream"))
			return
		}
		cursor = &core.EventCursor{After: after}
	} else if cursor, err = s.Svc.LatestEventCursor(ctx, p.Org.ID); err != nil {
		s.fail(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, fmt.Errorf("response writer cannot flush"))
		return
	}
	// A person's streams are counted per user, an agent's per key.
	owner := p.KeyID
	if owner == "" {
		owner = "user:" + p.UserID
	}
	if !s.streams.acquire(owner, cfg.PerKey) {
		s.fail(w, &core.Error{Status: http.StatusTooManyRequests, Name: "Too Many Requests", Code: "too_many_streams",
			Message: fmt.Sprintf("this caller already has %d open event streams", cfg.PerKey)})
		return
	}
	defer s.streams.release(owner)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // proxies that honour it do not buffer
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, "retry: 3000\n: connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	raw, _ := bearer(r)
	poll := time.NewTicker(cfg.Poll)
	defer poll.Stop()
	lastWrite, lastCheck := time.Now(), time.Now()
	for {
		for {
			events, err := s.Svc.NextEvents(ctx, p.Org.ID, filter, cursor, streamBatch)
			if err != nil {
				if ctx.Err() == nil {
					s.Log.Error("event stream poll failed", "err", err)
				}
				return // the client reconnects with Last-Event-ID
			}
			for _, e := range events {
				if err := writeEvent(w, e); err != nil {
					return
				}
			}
			if len(events) > 0 {
				flusher.Flush()
				lastWrite = time.Now()
			}
			if len(events) < streamBatch {
				break
			}
		}
		if time.Since(lastWrite) >= cfg.Heartbeat {
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
			lastWrite = time.Now()
		}
		if time.Since(lastCheck) >= cfg.Recheck {
			// A revoked key or an ended session loses its open streams too.
			if _, err := s.credential(ctx, raw); err != nil {
				_, _ = fmt.Fprint(w, "event: error\ndata: {\"code\":\"credential_invalid\"}\n\n")
				flusher.Flush()
				return
			}
			lastCheck = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-s.Done:
			return
		case <-poll.C:
		}
	}
}

func writeEvent(w http.ResponseWriter, e core.StreamEvent) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(dispatcher.Envelope{Type: "event", EventType: e.Type, EventID: e.EventID, CreatedAt: e.CreatedAt, Data: e.Payload}); err != nil {
		return err
	}
	// The encoder output is one line: JSON escapes newlines inside strings.
	_, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.Seq, e.Type, bytes.TrimRight(buf.Bytes(), "\n"))
	return err
}
