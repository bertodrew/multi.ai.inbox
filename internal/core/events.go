package core

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The event stream (GET /v0/events/stream) reads the same outbox as the
// webhook dispatcher, with the same payloads: an event about a message held
// by screening was written with its bodies redacted (emitEvent), so a stream
// consumer sees exactly what a webhook endpoint would see.

// StreamEvent is one outbox event for a stream consumer. Seq is the outbox
// order (events.id) and the Server Sent Events id used to resume.
type StreamEvent struct {
	Seq       int64
	EventID   string
	Type      string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// EventFilter narrows a stream to some event types and one inbox. Events
// without an inbox (domain.verified, organization.*) are left out by an
// inbox filter, as for a webhook with inbox_ids.
type EventFilter struct {
	Types   []string
	inboxID string
}

// NewEventFilter validates the types and resolves the inbox address.
func (s *Service) NewEventFilter(ctx context.Context, orgID string, types []string, address string) (EventFilter, error) {
	f := EventFilter{}
	for _, t := range types {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !EventTypes[t] {
			return f, Validation("unknown event type %q", t)
		}
		f.Types = append(f.Types, t)
	}
	if address == "" {
		return f, nil
	}
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		inbox, err := s.inboxTx(ctx, tx, address)
		if err == nil {
			f.inboxID = inbox.ID
		}
		return err
	})
	return f, err
}

// eventSettle bounds how long an outbox insert can stay invisible after a
// later one is already visible. events.id comes from a sequence, so a slow
// transaction can commit a lower id after a higher one was streamed; the
// cursor keeps looking below the newest id for this long.
const eventSettle = 30 * time.Second

// EventCursor tracks what a stream has delivered: every event up to After,
// plus the newer ids in recent that are still inside the settle window.
type EventCursor struct {
	After  int64
	recent map[int64]time.Time
}

// LatestEventCursor starts a stream at the newest event of the organization,
// so only events written from now on are delivered.
func (s *Service) LatestEventCursor(ctx context.Context, orgID string) (*EventCursor, error) {
	c := &EventCursor{}
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select coalesce(max(id), 0) from events").Scan(&c.After)
	})
	return c, err
}

// NextEvents returns up to limit undelivered events in outbox order and
// records them as delivered on c.
func (s *Service) NextEvents(ctx context.Context, orgID string, f EventFilter, c *EventCursor, limit int) ([]StreamEvent, error) {
	if c.recent == nil {
		c.recent = map[int64]time.Time{}
	}
	now := s.now()
	seen := make([]int64, 0, len(c.recent))
	for id := range c.recent {
		seen = append(seen, id)
	}
	var list []StreamEvent
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		q := `select id, event_id, type, payload, created_at from events
			where id > $1 and not (id = any($2::bigint[]))
			and (cardinality($3::text[]) = 0 or type = any($3::text[]))
			and ($4::uuid is null or inbox_id = $4::uuid)
			order by id limit $5`
		var inbox any
		if f.inboxID != "" {
			inbox = f.inboxID
		}
		rows, err := tx.Query(ctx, q, c.After, seen, nonNil(f.Types), inbox, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e StreamEvent
			if err := rows.Scan(&e.Seq, &e.EventID, &e.Type, &e.Payload, &e.CreatedAt); err != nil {
				return err
			}
			e.Payload = redactHeld(e.Payload)
			list = append(list, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	// Ids delivered longer than the settle window ago have nothing hidden
	// below them any more: fold them into After.
	for id, at := range c.recent {
		if now.Sub(at) > eventSettle && id > c.After {
			c.After = id
		}
	}
	for id := range c.recent {
		if id <= c.After {
			delete(c.recent, id)
		}
	}
	for _, e := range list {
		c.recent[e.Seq] = now
	}
	return list, nil
}

// redactHeld is defence in depth on top of emitEvent: a message payload that
// is still held by screening (pending or review) never carries body content.
func redactHeld(payload json.RawMessage) json.RawMessage {
	var data map[string]json.RawMessage
	if json.Unmarshal(payload, &data) != nil || data["message"] == nil {
		return payload
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(data["message"], &m) != nil {
		return payload
	}
	var screening string
	_ = json.Unmarshal(m["screening"], &screening)
	if screening != "pending" && screening != "review" {
		return payload
	}
	for _, k := range []string{"text", "html", "extracted_text"} {
		delete(m, k)
	}
	m["preview"] = json.RawMessage(`""`)
	data["message"], _ = json.Marshal(m)
	if t, ok := data["thread"]; ok {
		var th map[string]json.RawMessage
		if json.Unmarshal(t, &th) == nil {
			th["preview"] = json.RawMessage(`""`)
			data["thread"], _ = json.Marshal(th)
		}
	}
	out, err := json.Marshal(data)
	if err != nil {
		return payload
	}
	return out
}
