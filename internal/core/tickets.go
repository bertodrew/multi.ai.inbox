package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// A ticket is a thread with state: the message body is never copied, so the
// queue and the mailbox can never disagree. Design and review:
// docs/agentmail-re/15-ticketing.md and 16-ticketing-review.md.
const (
	TicketNew      = "new"
	TicketOpen     = "open"
	TicketPending  = "pending"
	TicketResolved = "resolved"
	TicketClosed   = "closed"
)

const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

type Ticket struct {
	ID            string     `json:"id"`
	Number        int64      `json:"number"`
	InboxID       string     `json:"inbox_id"`
	ThreadID      string     `json:"thread_id"`
	Subject       string     `json:"subject"`
	Requester     string     `json:"requester"`
	State         string     `json:"state"`
	Priority      string     `json:"priority"`
	AssigneeUser  string     `json:"assignee_user,omitempty"`
	AssigneeKey   string     `json:"assignee_key,omitempty"`
	SLAPolicy     string     `json:"sla_policy,omitempty"`
	DueFirstReply *time.Time `json:"due_first_reply,omitempty"`
	DueResolution *time.Time `json:"due_resolution,omitempty"`
	FirstReplyAt  *time.Time `json:"first_reply_at,omitempty"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
	Tags          []string   `json:"tags"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Breaching reports whether the first reply SLA is already missed.
func (t *Ticket) Breaching(now time.Time) bool {
	return t.DueFirstReply != nil && t.FirstReplyAt == nil &&
		(t.State == TicketNew || t.State == TicketOpen) && now.After(*t.DueFirstReply)
}

type TicketEvent struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Actor     string          `json:"actor"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt time.Time       `json:"created_at"`
}

type TicketNote struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type TicketStats struct {
	ByState                 map[string]int `json:"by_state"`
	Open                    int            `json:"open"`
	Breaching               int            `json:"breaching"`
	Unassigned              int            `json:"unassigned"`
	MedianFirstReplySeconds int64          `json:"median_first_reply_seconds"`
}

type CreateTicketRequest struct {
	ThreadID     string   `json:"thread_id"`
	Priority     string   `json:"priority,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	AssigneeUser string   `json:"assignee_user,omitempty"`
	AssigneeKey  string   `json:"assignee_key,omitempty"`
}

type UpdateTicketRequest struct {
	State        *string   `json:"state,omitempty"`
	Priority     *string   `json:"priority,omitempty"`
	Tags         *[]string `json:"tags,omitempty"`
	AssigneeUser *string   `json:"assignee_user,omitempty"`
	AssigneeKey  *string   `json:"assignee_key,omitempty"`
}

type AddNoteRequest struct {
	Body string `json:"body"`
}

// TicketFilter is the queue query. Empty fields mean "no filter".
type TicketFilter struct {
	State    string
	Priority string
	Assignee string // a user id, a key id, or "none"
	Inbox    string // inbox address
	Query    string
}

const ticketCols = `id::text, number, inbox_id::text, thread_id::text, subject, requester, state, priority,
	coalesce(assignee_user::text, ''), coalesce(assignee_key::text, ''), coalesce(sla_policy, ''),
	due_first_reply, due_resolution, first_reply_at, resolved_at, closed_at, tags, created_at, updated_at`

func scanTicket(row pgx.Row) (*Ticket, error) {
	var t Ticket
	if err := row.Scan(&t.ID, &t.Number, &t.InboxID, &t.ThreadID, &t.Subject, &t.Requester, &t.State,
		&t.Priority, &t.AssigneeUser, &t.AssigneeKey, &t.SLAPolicy, &t.DueFirstReply, &t.DueResolution,
		&t.FirstReplyAt, &t.ResolvedAt, &t.ClosedAt, &t.Tags, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func validState(s string) bool {
	switch s {
	case TicketNew, TicketOpen, TicketPending, TicketResolved, TicketClosed:
		return true
	}
	return false
}

func validPriority(p string) bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

// transitionAllowed keeps the state machine explicit: an agent cannot jump from
// closed to pending, and nothing moves on its own.
func transitionAllowed(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case TicketNew:
		return to == TicketOpen || to == TicketPending || to == TicketResolved || to == TicketClosed
	case TicketOpen:
		return to == TicketPending || to == TicketResolved || to == TicketClosed
	case TicketPending:
		return to == TicketOpen || to == TicketResolved || to == TicketClosed
	case TicketResolved:
		return to == TicketOpen || to == TicketClosed
	case TicketClosed:
		return to == TicketOpen
	}
	return false
}

// CreateTicket opens a ticket on an existing thread. The body is not copied:
// the ticket only points at the thread.
func (s *Service) CreateTicket(ctx context.Context, p *Principal, req CreateTicketRequest) (*Ticket, error) {
	if strings.TrimSpace(req.ThreadID) == "" {
		return nil, errf(http.StatusBadRequest, "thread_required", "thread_id is required")
	}
	priority := req.Priority
	if priority == "" {
		priority = PriorityNormal
	}
	if !validPriority(priority) {
		return nil, errf(http.StatusBadRequest, "invalid_priority", "priority must be low, normal, high or urgent")
	}
	var out *Ticket
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		var inboxID, subject, requester string
		err := tx.QueryRow(ctx, `select t.inbox_id::text, t.subject,
			coalesce((select m.from_addr from messages m
			          where m.thread_id = t.id and m.direction = 'in'
			          order by m.created_at asc limit 1), '')
			from threads t where t.org_id = $1 and t.id = $2`, p.Org.ID, req.ThreadID).
			Scan(&inboxID, &subject, &requester)
		if errors.Is(err, pgx.ErrNoRows) {
			return errf(http.StatusNotFound, "thread_not_found", "thread %s not found", req.ThreadID)
		}
		if err != nil {
			return err
		}
		number, err := s.nextTicketNumber(ctx, tx, p.Org.ID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		first, resolution := slaDue(now, p.Org.Plan, priority)
		row := tx.QueryRow(ctx, `insert into tickets
			(org_id, number, inbox_id, thread_id, subject, requester, state, priority,
			 assignee_user, assignee_key, sla_policy, due_first_reply, due_resolution, tags)
			values ($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,'')::uuid,nullif($10,'')::uuid,$11,$12,$13,$14)
			returning `+ticketCols,
			p.Org.ID, number, inboxID, req.ThreadID, subject, requester, TicketNew, priority,
			req.AssigneeUser, req.AssigneeKey, p.Org.Plan, first, resolution, nonNil(req.Tags))
		out, err = scanTicket(row)
		if err != nil {
			if isUniqueViolation(err) {
				return errf(http.StatusConflict, "ticket_exists", "this thread already has a ticket")
			}
			return err
		}
		return s.ticketEvent(ctx, tx, p, out.ID, "created",
			map[string]any{"state": out.State, "priority": out.Priority})
	})
	return out, err
}

// nextTicketNumber allocates the per-organization number. An advisory lock on
// the organization keeps two concurrent creations from picking the same number
// without needing write access to the organizations row.
func (s *Service) nextTicketNumber(ctx context.Context, tx pgx.Tx, orgID string) (int64, error) {
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtext($1))`, "tickets:"+orgID); err != nil {
		return 0, err
	}
	var n int64
	err := tx.QueryRow(ctx, `select coalesce(max(number), 0) + 1 from tickets where org_id = $1`, orgID).Scan(&n)
	return n, err
}

// ListTickets returns the queue, newest activity first. The page token is a
// plain offset: a queue is small and the ordering is by updated_at.
func (s *Service) ListTickets(ctx context.Context, p *Principal, f TicketFilter, page Page) ([]*Ticket, string, error) {
	offset, _ := strconv.Atoi(page.Token)
	sql := `select ` + ticketCols + ` from tickets where org_id = $1`
	args := []any{p.Org.ID}
	add := func(clause string, v any) {
		args = append(args, v)
		sql += " and " + clause + " $" + strconv.Itoa(len(args))
	}
	if f.State != "" {
		if !validState(f.State) {
			return nil, "", errf(http.StatusBadRequest, "invalid_state", "unknown state %q", f.State)
		}
		add("state =", f.State)
	}
	if f.Priority != "" {
		add("priority =", f.Priority)
	}
	if f.Assignee != "" {
		if f.Assignee == "none" {
			sql += " and assignee_user is null and assignee_key is null"
		} else {
			args = append(args, f.Assignee)
			n := strconv.Itoa(len(args))
			sql += " and (assignee_user::text = $" + n + " or assignee_key::text = $" + n + ")"
		}
	}
	if f.Inbox != "" {
		args = append(args, f.Inbox)
		n := strconv.Itoa(len(args))
		sql += " and inbox_id = (select id from inboxes where address = $" + n + " and org_id = $1)"
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		n := strconv.Itoa(len(args))
		sql += " and (subject ilike $" + n + " or requester ilike $" + n + ")"
	}
	sql += " order by updated_at desc, id desc"
	args = append(args, page.limit()+1, offset)
	sql += " limit $" + strconv.Itoa(len(args)-1) + " offset $" + strconv.Itoa(len(args))

	out := make([]*Ticket, 0, page.limit())
	next := ""
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanTicket(rows)
			if err != nil {
				return err
			}
			out = append(out, t)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) > page.limit() {
			out = out[:page.limit()]
			next = strconv.Itoa(offset + page.limit())
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return out, next, nil
}

// GetTicket returns the ticket with its timeline and notes.
func (s *Service) GetTicket(ctx context.Context, p *Principal, id string) (*Ticket, []*TicketEvent, []*TicketNote, error) {
	var t *Ticket
	var events []*TicketEvent
	var notes []*TicketNote
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		var err error
		t, err = scanTicket(tx.QueryRow(ctx, `select `+ticketCols+` from tickets where org_id = $1 and id = $2`, p.Org.ID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return errf(http.StatusNotFound, "ticket_not_found", "ticket %s not found", id)
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `select id, kind, coalesce(actor_user::text,''), coalesce(actor_key::text,''), detail, created_at
			from ticket_events where ticket_id = $1 order by id`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e TicketEvent
			var user, key string
			if err := rows.Scan(&e.ID, &e.Kind, &user, &key, &e.Detail, &e.CreatedAt); err != nil {
				return err
			}
			e.Actor = ticketActor(user, key)
			events = append(events, &e)
		}
		nrows, err := tx.Query(ctx, `select id::text, coalesce(author_user::text,''), coalesce(author_key::text,''), body, created_at
			from ticket_notes where ticket_id = $1 order by created_at`, id)
		if err != nil {
			return err
		}
		defer nrows.Close()
		for nrows.Next() {
			var n TicketNote
			var user, key string
			if err := nrows.Scan(&n.ID, &user, &key, &n.Body, &n.CreatedAt); err != nil {
				return err
			}
			n.Author = ticketActor(user, key)
			notes = append(notes, &n)
		}
		return nrows.Err()
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return t, events, notes, nil
}

// UpdateTicket applies an explicit transition, a priority change, tags or an
// assignment. Every change writes a timeline row.
func (s *Service) UpdateTicket(ctx context.Context, p *Principal, id string, req UpdateTicketRequest) (*Ticket, error) {
	var out *Ticket
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		cur, err := scanTicket(tx.QueryRow(ctx, `select `+ticketCols+` from tickets where org_id = $1 and id = $2`, p.Org.ID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return errf(http.StatusNotFound, "ticket_not_found", "ticket %s not found", id)
		}
		if err != nil {
			return err
		}
		state, priority := cur.State, cur.Priority
		if req.State != nil {
			if !validState(*req.State) {
				return errf(http.StatusBadRequest, "invalid_state", "unknown state %q", *req.State)
			}
			if !transitionAllowed(cur.State, *req.State) {
				return errf(http.StatusConflict, "invalid_transition", "cannot move a ticket from %s to %s", cur.State, *req.State)
			}
			state = *req.State
		}
		if req.Priority != nil {
			if !validPriority(*req.Priority) {
				return errf(http.StatusBadRequest, "invalid_priority", "unknown priority %q", *req.Priority)
			}
			priority = *req.Priority
		}
		// A priority change recomputes the SLA from now, so the promise follows
		// the urgency the requester actually got.
		var first, resolution *time.Time
		if priority != cur.Priority {
			f, r := slaDue(time.Now().UTC(), p.Org.Plan, priority)
			first, resolution = f, r
		}
		var tags []string
		if req.Tags != nil {
			tags = nonNil(*req.Tags)
		}
		row := tx.QueryRow(ctx, `update tickets set
				state = $3, priority = $4,
				assignee_user = case when $5::boolean then nullif($6,'')::uuid else assignee_user end,
				assignee_key = case when $7::boolean then nullif($8,'')::uuid else assignee_key end,
				tags = coalesce($9, tags),
				resolved_at = case when $3 = 'resolved' and resolved_at is null then now() else resolved_at end,
				closed_at = case when $3 = 'closed' and closed_at is null then now() else closed_at end,
				due_first_reply = coalesce($10, due_first_reply),
				due_resolution = coalesce($11, due_resolution),
				updated_at = now()
			where org_id = $1 and id = $2
			returning `+ticketCols,
			p.Org.ID, id, state, priority,
			req.AssigneeUser != nil, deref(req.AssigneeUser),
			req.AssigneeKey != nil, deref(req.AssigneeKey),
			tags, first, resolution)
		out, err = scanTicket(row)
		if err != nil {
			return err
		}
		if state != cur.State {
			if err := s.ticketEvent(ctx, tx, p, id, "state", map[string]any{"from": cur.State, "to": state}); err != nil {
				return err
			}
		}
		if priority != cur.Priority {
			if err := s.ticketEvent(ctx, tx, p, id, "priority", map[string]any{"from": cur.Priority, "to": priority}); err != nil {
				return err
			}
		}
		if req.AssigneeUser != nil || req.AssigneeKey != nil {
			if err := s.ticketEvent(ctx, tx, p, id, "assigned", map[string]any{
				"user": out.AssigneeUser, "key": out.AssigneeKey}); err != nil {
				return err
			}
		}
		if req.Tags != nil {
			if err := s.ticketEvent(ctx, tx, p, id, "tags", map[string]any{"tags": out.Tags}); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// AddTicketNote stores an internal note. Notes live in their own table and have
// no code path to the sender: a note can never be emailed by mistake.
func (s *Service) AddTicketNote(ctx context.Context, p *Principal, id string, req AddNoteRequest) (*TicketNote, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, errf(http.StatusBadRequest, "note_required", "a note needs a body")
	}
	var out *TicketNote
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from tickets where org_id = $1 and id = $2)`,
			p.Org.ID, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errf(http.StatusNotFound, "ticket_not_found", "ticket %s not found", id)
		}
		var noteID, user, key, noteBody string
		var createdAt time.Time
		err := tx.QueryRow(ctx, `insert into ticket_notes (org_id, ticket_id, author_user, author_key, body)
			values ($1,$2,nullif($3,'')::uuid,nullif($4,'')::uuid,$5)
			returning id::text, coalesce(author_user::text,''), coalesce(author_key::text,''), body, created_at`,
			p.Org.ID, id, p.UserID, p.KeyID, body).
			Scan(&noteID, &user, &key, &noteBody, &createdAt)
		if err != nil {
			return err
		}
		out = &TicketNote{ID: noteID, Author: ticketActor(user, key), Body: noteBody, CreatedAt: createdAt}
		return s.ticketEvent(ctx, tx, p, id, "note", map[string]any{"note_id": noteID})
	})
	return out, err
}

// ReplyTicket sends the answer through the normal mail path and records the
// first reply time, which is what the SLA measures.
func (s *Service) ReplyTicket(ctx context.Context, p *Principal, id string, req ReplyRequest, idemKey string) (*SendResult, error) {
	var address, threadID string
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select i.address, t.thread_id::text from tickets t
			join inboxes i on i.id = t.inbox_id where t.org_id = $1 and t.id = $2`, p.Org.ID, id).
			Scan(&address, &threadID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errf(http.StatusNotFound, "ticket_not_found", "ticket %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	parent := lastInboundMessage(ctx, s, p.Org.ID, threadID)
	if parent == "" {
		return nil, errf(http.StatusConflict, "no_inbound_message", "this ticket has no inbound message to reply to")
	}
	res, err := s.Reply(ctx, p.Org.ID, address, parent, req, idemKey)
	if err != nil {
		return nil, err
	}
	_ = s.markTicketReply(ctx, p, id)
	return res, nil
}

func lastInboundMessage(ctx context.Context, s *Service, orgID, threadID string) string {
	var id string
	_ = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select rfc_message_id from messages
			where org_id = $1 and thread_id = $2 and direction = 'in' order by created_at desc limit 1`,
			orgID, threadID).Scan(&id)
	})
	return id
}

func (s *Service) markTicketReply(ctx context.Context, p *Principal, id string) error {
	return s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		var firstReply bool
		err := tx.QueryRow(ctx, `update tickets set first_reply_at = coalesce(first_reply_at, now()),
			state = case when state = 'new' then 'open' else state end, updated_at = now()
			where org_id = $1 and id = $2 and first_reply_at is null returning true`, p.Org.ID, id).Scan(&firstReply)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already answered
		}
		if err != nil {
			return err
		}
		return s.ticketEvent(ctx, tx, p, id, "reply", map[string]any{})
	})
}

// TicketStats feeds the queue header.
func (s *Service) TicketStats(ctx context.Context, p *Principal) (*TicketStats, error) {
	out := &TicketStats{ByState: map[string]int{}}
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select state, count(*) from tickets where org_id = $1 group by state`, p.Org.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var state string
			var n int
			if err := rows.Scan(&state, &n); err != nil {
				return err
			}
			out.ByState[state] = n
		}
		out.Open = out.ByState[TicketNew] + out.ByState[TicketOpen] + out.ByState[TicketPending]
		return tx.QueryRow(ctx, `select
			count(*) filter (where assignee_user is null and assignee_key is null
			                 and state in ('new','open','pending')),
			count(*) filter (where first_reply_at is null and due_first_reply is not null
			                 and due_first_reply < now() and state in ('new','open')),
			coalesce((select percentile_cont(0.5) within group (
			             order by extract(epoch from first_reply_at - created_at))
			          from tickets where org_id = $1 and first_reply_at is not null), 0)::bigint
			from tickets where org_id = $1`, p.Org.ID).Scan(&out.Unassigned, &out.Breaching, &out.MedianFirstReplySeconds)
	})
	return out, err
}

// IntakeTicketForMessage opens the ticket for a message that has just become
// visible: the screening verdict allowed it, or a human released it. Held mail
// never opens a ticket, so the queue cannot list a thread the reader cannot
// open. One ticket per thread, so a later reply is a no-op.
func (s *Service) IntakeTicketForMessage(ctx context.Context, tx pgx.Tx, messageID string) error {
	var orgID, inboxID, threadID, subject, requester string
	err := tx.QueryRow(ctx, `select org_id::text, inbox_id::text, thread_id::text, subject, from_addr
		from messages where id = $1`, messageID).Scan(&orgID, &inboxID, &threadID, &subject, &requester)
	if err != nil {
		return err
	}
	return s.IntakeTicket(ctx, tx, orgID, inboxID, threadID, subject, strings.ToLower(requester))
}

// IntakeTicket creates a ticket for a thread in an inbox marked as intake. It is
// called from the ingest path, inside the same transaction, and does nothing for
// an inbox that is not an intake.
func (s *Service) IntakeTicket(ctx context.Context, tx pgx.Tx, orgID, inboxID, threadID, subject, requester string) error {
	var intake bool
	if err := tx.QueryRow(ctx, `select ticket_intake from inboxes where id = $1`, inboxID).Scan(&intake); err != nil {
		return err
	}
	if !intake {
		return nil
	}
	var plan string
	if err := tx.QueryRow(ctx, `select coalesce(plan,'free') from organizations where id = $1`, orgID).Scan(&plan); err != nil {
		return err
	}
	number, err := s.nextTicketNumber(ctx, tx, orgID)
	if err != nil {
		return err
	}
	first, resolution := slaDue(time.Now().UTC(), plan, PriorityNormal)
	var id string
	err = tx.QueryRow(ctx, `insert into tickets
		(org_id, number, inbox_id, thread_id, subject, requester, state, priority, sla_policy, due_first_reply, due_resolution)
		values ($1,$2,$3,$4,$5,$6,'new','normal',$7,$8,$9)
		on conflict (thread_id) do nothing
		returning id::text`,
		orgID, number, inboxID, threadID, subject, requester, plan, first, resolution).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // a ticket already exists for this thread
	}
	if err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]any{"state": TicketNew, "priority": PriorityNormal, "source": "intake"})
	_, err = tx.Exec(ctx, `insert into ticket_events (org_id, ticket_id, kind, detail) values ($1,$2,'created',$3)`,
		orgID, id, detail)
	return err
}

func (s *Service) ticketEvent(ctx context.Context, tx pgx.Tx, p *Principal, ticketID, kind string, detail map[string]any) error {
	raw, _ := json.Marshal(detail)
	_, err := tx.Exec(ctx, `insert into ticket_events (org_id, ticket_id, kind, actor_user, actor_key, detail)
		values ($1,$2,$3,nullif($4,'')::uuid,nullif($5,'')::uuid,$6)`,
		p.Org.ID, ticketID, kind, p.UserID, p.KeyID, raw)
	return err
}

func ticketActor(user, key string) string {
	if user != "" {
		return "user:" + user
	}
	if key != "" {
		return "api_key:" + key
	}
	return "system"
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SLA in business hours: Monday to Friday, 09:00 to 18:00 CET. The table lives
// in code so it is reviewable next to the pricing page.
var slaTable = map[string]struct {
	firstReply time.Duration
	resolution time.Duration
}{
	"free":    {0, 0},
	"builder": {24 * time.Hour, 45 * time.Hour}, // 5 business days
	"growth":  {8 * time.Hour, 27 * time.Hour},  // 3 business days
	"team":    {4 * time.Hour, 18 * time.Hour},  // 2 business days
}

var priorityFactor = map[string]float64{
	PriorityUrgent: 0.25,
	PriorityHigh:   0.5,
	PriorityNormal: 1,
	PriorityLow:    2,
}

var cetZone = time.FixedZone("CET", 60*60)

// holidays is empty until the founder provides the list for the year.
var holidays = map[string]bool{}

func slaDue(now time.Time, plan, priority string) (*time.Time, *time.Time) {
	policy, ok := slaTable[plan]
	if !ok {
		policy = slaTable["free"]
	}
	factor := priorityFactor[priority]
	if factor == 0 {
		factor = 1
	}
	if policy.firstReply == 0 {
		return nil, nil
	}
	first := addBusiness(now, time.Duration(float64(policy.firstReply)*factor))
	res := addBusiness(now, time.Duration(float64(policy.resolution)*factor))
	return &first, &res
}

// addBusiness advances d inside business hours only.
func addBusiness(start time.Time, d time.Duration) time.Time {
	if d <= 0 {
		return start
	}
	t := nextBusinessStart(start)
	for d > 0 {
		end := time.Date(t.Year(), t.Month(), t.Day(), 18, 0, 0, 0, cetZone)
		available := end.Sub(t)
		if d <= available {
			return t.Add(d)
		}
		d -= available
		t = nextBusinessStart(time.Date(t.Year(), t.Month(), t.Day()+1, 9, 0, 0, 0, cetZone))
	}
	return t
}

func nextBusinessStart(t time.Time) time.Time {
	t = t.In(cetZone)
	for i := 0; i < 400; i++ { // bounded: at most about a year of closed days
		day := t.Format("2006-01-02")
		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday || holidays[day] {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 9, 0, 0, 0, cetZone)
			continue
		}
		hour := t.Hour()
		if hour < 9 {
			return time.Date(t.Year(), t.Month(), t.Day(), 9, 0, 0, 0, cetZone)
		}
		if hour >= 18 {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 9, 0, 0, 0, cetZone)
			continue
		}
		return t
	}
	return t
}
