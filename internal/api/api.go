// Package api is the REST surface (/v0). Handlers only decode, validate the
// shape of the request and call core; all rules live in core.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

type ctxKey struct{}

// OrgFrom returns the authenticated organization of a request.
func OrgFrom(ctx context.Context) *core.Org {
	o, _ := ctx.Value(ctxKey{}).(*core.Org)
	return o
}

type Server struct {
	Svc *core.Service
	Log *slog.Logger
	MCP http.Handler // optional, mounted at /mcp behind the same auth
}

const maxBody = 8 << 20

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	auth := http.NewServeMux()
	auth.HandleFunc("GET /v0/auth/me", s.me)
	auth.HandleFunc("POST /v0/inboxes", s.createInbox)
	auth.HandleFunc("GET /v0/inboxes", s.listInboxes)
	auth.HandleFunc("GET /v0/inboxes/{inbox}", s.getInbox)
	auth.HandleFunc("PATCH /v0/inboxes/{inbox}", s.updateInbox)
	auth.HandleFunc("DELETE /v0/inboxes/{inbox}", s.deleteInbox)
	auth.HandleFunc("GET /v0/inboxes/{inbox}/threads", s.listThreads)
	auth.HandleFunc("GET /v0/inboxes/{inbox}/threads/{thread}", s.getThread)
	auth.HandleFunc("GET /v0/threads", s.listThreads)
	auth.HandleFunc("GET /v0/threads/{thread}", s.getThread)
	auth.HandleFunc("GET /v0/inboxes/{inbox}/messages", s.listMessages)
	auth.HandleFunc("POST /v0/inboxes/{inbox}/messages/send", s.send)
	auth.HandleFunc("GET /v0/inboxes/{inbox}/messages/{message}", s.getMessage)
	auth.HandleFunc("PATCH /v0/inboxes/{inbox}/messages/{message}", s.updateMessage)
	auth.HandleFunc("GET /v0/inboxes/{inbox}/messages/{message}/raw", s.rawMessage)
	auth.HandleFunc("POST /v0/inboxes/{inbox}/messages/{message}/reply", s.reply)
	auth.HandleFunc("POST /v0/webhooks", s.createWebhook)
	auth.HandleFunc("GET /v0/webhooks", s.listWebhooks)
	auth.HandleFunc("DELETE /v0/webhooks/{webhook}", s.deleteWebhook)
	if s.MCP != nil {
		auth.Handle("/mcp", s.MCP)
	}
	mux.Handle("/v0/", s.authenticate(auth))
	mux.Handle("/mcp", s.authenticate(auth))
	return mux
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			key = strings.TrimPrefix(h, "Bearer ")
		} else if h := r.Header.Get("X-Api-Key"); h != "" {
			key = h
		}
		if key == "" {
			s.fail(w, &core.Error{Status: http.StatusUnauthorized, Name: "Unauthorized", Code: "missing_authorization",
				Message: "send Authorization: Bearer <api key>"})
			return
		}
		org, err := s.Svc.Authenticate(r.Context(), key)
		if err != nil {
			s.fail(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, org)))
	})
}

func (s *Server) ok(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.Log.Warn("write response", "err", err)
	}
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	e := core.AsError(err)
	if e.Status >= 500 {
		s.Log.Error("request failed", "err", err)
	}
	if e.Status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	s.ok(w, e.Status, e)
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return core.Validation("request body is required")
		}
		return core.Validation("invalid JSON body: %v", err)
	}
	return nil
}

func page(r *http.Request) core.Page {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return core.Page{Limit: limit, Token: r.URL.Query().Get("page_token")}
}

func listBody[T any](name string, items []T, next string) map[string]any {
	if items == nil {
		items = []T{}
	}
	body := map[string]any{"count": len(items), name: items}
	if next != "" {
		body["next_page_token"] = next
	}
	return body
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	s.ok(w, http.StatusOK, OrgFrom(r.Context()))
}

func (s *Server) createInbox(w http.ResponseWriter, r *http.Request) {
	var req core.CreateInboxRequest
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, err)
			return
		}
	}
	inbox, created, err := s.Svc.CreateInbox(r.Context(), OrgFrom(r.Context()).ID, req)
	if err != nil {
		s.fail(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	s.ok(w, status, inbox)
}

func (s *Server) listInboxes(w http.ResponseWriter, r *http.Request) {
	items, next, err := s.Svc.ListInboxes(r.Context(), OrgFrom(r.Context()).ID, page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("inboxes", items, next))
}

func (s *Server) getInbox(w http.ResponseWriter, r *http.Request) {
	inbox, err := s.Svc.GetInbox(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, inbox)
}

func (s *Server) updateInbox(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName *string `json:"display_name"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	inbox, err := s.Svc.UpdateInbox(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), req.DisplayName)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, inbox)
}

func (s *Server) deleteInbox(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.DeleteInbox(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listThreads(w http.ResponseWriter, r *http.Request) {
	items, next, err := s.Svc.ListThreads(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), page(r), r.URL.Query().Get("label"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("threads", items, next))
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request) {
	t, err := s.Svc.GetThread(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("thread"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if inbox := r.PathValue("inbox"); inbox != "" && !strings.EqualFold(inbox, t.InboxID) {
		s.fail(w, core.NotFound("thread"))
		return
	}
	s.ok(w, http.StatusOK, t)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	items, next, err := s.Svc.ListMessages(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), page(r), r.URL.Query().Get("label"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("messages", items, next))
}

func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	m, err := s.Svc.GetMessage(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("message"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, m)
}

func (s *Server) updateMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AddLabels    []string `json:"add_labels"`
		RemoveLabels []string `json:"remove_labels"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	m, err := s.Svc.UpdateMessageLabels(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("message"), req.AddLabels, req.RemoveLabels)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, m)
}

func (s *Server) rawMessage(w http.ResponseWriter, r *http.Request) {
	raw, err := s.Svc.RawMessage(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("message"))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "message/rfc822")
	_, _ = w.Write(raw)
}

func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	var req core.SendRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.Svc.Send(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), req, r.Header.Get("Idempotency-Key"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, res)
}

func (s *Server) reply(w http.ResponseWriter, r *http.Request) {
	var req core.ReplyRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.Svc.Reply(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("message"), req, r.Header.Get("Idempotency-Key"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, res)
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	var req core.CreateWebhookRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	hook, err := s.Svc.CreateWebhook(r.Context(), OrgFrom(r.Context()).ID, req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusCreated, hook)
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	items, err := s.Svc.ListWebhooks(r.Context(), OrgFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("webhooks", items, ""))
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.DeleteWebhook(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("webhook")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
