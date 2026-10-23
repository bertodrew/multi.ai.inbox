package aimulti

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Event is one server sent event: its position in the stream, its type and the
// webhook envelope as JSON.
type Event struct {
	// ID is the position in the stream, for resuming with EventOptions.
	ID string
	// Name is the event type, for example "message.received".
	Name string
	// Data is the webhook envelope: type, event_type, event_id, created_at,
	// data. Payloads of events about messages held by screening carry no body.
	Data []byte
}

// Envelope decodes Data into the webhook envelope.
func (e Event) Envelope() (*EventEnvelope, error) {
	out := &EventEnvelope{}
	if err := json.Unmarshal(e.Data, out); err != nil {
		return nil, fmt.Errorf("aimulti: decode event %s: %w", e.ID, err)
	}
	return out, nil
}

// EventStream is an open event stream. Call Close when done; it unblocks a
// pending Next.
type EventStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
}

// Events opens the organization's event stream. The caller must close it.
func (c *Client) Events(ctx context.Context, opts EventOptions) (*EventStream, error) {
	path := "/events/stream"
	u := c.baseURL + "/" + c.version + path
	if q := opts.values(); len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("aimulti: build stream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", c.userAgent)
	if opts.LastEventID > 0 {
		req.Header.Set("Last-Event-ID", itoa64(opts.LastEventID))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aimulti: open stream: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
		return nil, apiError(resp, raw, http.MethodGet, u)
	}
	return &EventStream{body: resp.Body, reader: bufio.NewReader(resp.Body)}, nil
}

// Next returns the next event, skipping keep alive comments. It blocks until an
// event arrives or the stream ends, at which point it returns io.EOF. Close the
// stream to unblock it.
func (s *EventStream) Next() (Event, error) {
	var e Event
	var data []string
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && (e.ID != "" || e.Name != "" || len(data) > 0) {
				e.Data = []byte(strings.Join(data, "\n"))
				return e, nil
			}
			return Event{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if e.ID != "" || e.Name != "" || len(data) > 0 {
				e.Data = []byte(strings.Join(data, "\n"))
				return e, nil
			}
			continue // an empty block, or a block that only held comments
		}
		if strings.HasPrefix(line, ":") {
			continue // keep alive comment
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			e.ID = value
		case "event":
			e.Name = value
		case "data":
			data = append(data, value)
		}
	}
}

// Close ends the stream.
func (s *EventStream) Close() error { return s.body.Close() }
