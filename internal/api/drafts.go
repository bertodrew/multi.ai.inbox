package api

import (
	"net/http"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

func (s *Server) createDraft(w http.ResponseWriter, r *http.Request) {
	var req core.CreateDraftRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	d, err := s.Svc.CreateDraft(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusCreated, d)
}

func (s *Server) listDrafts(w http.ResponseWriter, r *http.Request) {
	items, next, err := s.Svc.ListDrafts(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("drafts", items, next))
}

func (s *Server) getDraft(w http.ResponseWriter, r *http.Request) {
	d, err := s.Svc.GetDraft(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("draft"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, d)
}

func (s *Server) updateDraft(w http.ResponseWriter, r *http.Request) {
	var req core.UpdateDraftRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	d, err := s.Svc.UpdateDraft(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("draft"), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, d)
}

func (s *Server) deleteDraft(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.DeleteDraft(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("draft")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sendDraft(w http.ResponseWriter, r *http.Request) {
	res, err := s.Svc.SendDraft(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("draft"),
		r.Header.Get("Idempotency-Key"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, res)
}
