package api

import (
	"net/http"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

// listTickets is the queue: filters come from the query string and the page
// token is a plain offset, which is enough for a queue ordered by activity.
func (s *Server) listTickets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := core.TicketFilter{
		State:    q.Get("state"),
		Priority: q.Get("priority"),
		Assignee: q.Get("assignee"),
		Inbox:    q.Get("inbox"),
		Query:    q.Get("q"),
	}
	items, next, err := s.Svc.ListTickets(r.Context(), PrincipalFrom(r.Context()), f, page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("tickets", items, next))
}

func (s *Server) createTicket(w http.ResponseWriter, r *http.Request) {
	var req core.CreateTicketRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	t, err := s.Svc.CreateTicket(r.Context(), PrincipalFrom(r.Context()), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "ticket.create", "ticket:"+t.ID, map[string]any{"number": t.Number, "thread": t.ThreadID})
	s.ok(w, http.StatusCreated, t)
}

func (s *Server) getTicket(w http.ResponseWriter, r *http.Request) {
	t, events, notes, err := s.Svc.GetTicket(r.Context(), PrincipalFrom(r.Context()), r.PathValue("ticket"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"ticket": t, "events": events, "notes": notes})
}

func (s *Server) updateTicket(w http.ResponseWriter, r *http.Request) {
	var req core.UpdateTicketRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	t, err := s.Svc.UpdateTicket(r.Context(), PrincipalFrom(r.Context()), r.PathValue("ticket"), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "ticket.update", "ticket:"+t.ID, map[string]any{"state": t.State, "priority": t.Priority})
	s.ok(w, http.StatusOK, t)
}

func (s *Server) addTicketNote(w http.ResponseWriter, r *http.Request) {
	var req core.AddNoteRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	n, err := s.Svc.AddTicketNote(r.Context(), PrincipalFrom(r.Context()), r.PathValue("ticket"), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "ticket.note", "ticket:"+r.PathValue("ticket"), map[string]any{"note": n.ID})
	s.ok(w, http.StatusCreated, n)
}

// replyTicket answers the customer through the normal reply path, so threading
// and the gatekeeper behave exactly as they do for any other reply.
func (s *Server) replyTicket(w http.ResponseWriter, r *http.Request) {
	var req core.ReplyRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.Svc.ReplyTicket(r.Context(), PrincipalFrom(r.Context()), r.PathValue("ticket"), req,
		r.Header.Get("Idempotency-Key"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "ticket.reply", "ticket:"+r.PathValue("ticket"), map[string]any{"message": res.MessageID})
	s.ok(w, http.StatusOK, res)
}

func (s *Server) ticketStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.Svc.TicketStats(r.Context(), PrincipalFrom(r.Context()))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, st)
}
