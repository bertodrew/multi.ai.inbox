// Package api is the REST surface (/v0). Handlers only decode, validate the
// shape of the request and call core; all rules live in core.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/openapi"
)

type ctxKey struct{}

// PrincipalFrom returns the authenticated caller of a request.
func PrincipalFrom(ctx context.Context) *core.Principal {
	p, _ := ctx.Value(ctxKey{}).(*core.Principal)
	return p
}

// OrgFrom returns the authenticated organization of a request.
func OrgFrom(ctx context.Context) *core.Org {
	if p := PrincipalFrom(ctx); p != nil {
		return p.Org
	}
	return nil
}

type Server struct {
	Svc *core.Service
	Log *slog.Logger
	MCP http.Handler // optional, mounted at /mcp behind the same auth
	// Limiter caps requests per organization; nil disables it.
	Limiter *Limiter
	// AuthFailures caps failed authentications per client IP (brute force); nil disables it.
	AuthFailures *Limiter
	// Gateway holds the edge settings (CORS, trusted proxies).
	Gateway Gateway
	// Streams caps the open event streams per API key; zero values use the defaults.
	Streams StreamSettings
	// Done ends open event streams (server shutdown); nil means never.
	Done <-chan struct{}

	streams streamCounter
}

const maxBody = 8 << 20

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Svc.DB.Pool.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	auth := http.NewServeMux()
	route := func(pattern, perm string, h http.HandlerFunc) { auth.Handle(pattern, s.require(perm, h)) }
	route("POST /v0/api-keys", "", s.createKey)
	route("GET /v0/api-keys", "", s.listKeys)
	route("DELETE /v0/api-keys/{key}", "", s.revokeKey)
	route("GET /v0/audit-logs", "", s.listAudit)
	route("GET /v0/auth/me", "", s.me)
	route("POST /v0/inboxes", core.PermInboxWrite, s.createInbox)
	route("GET /v0/inboxes", core.PermInboxRead, s.listInboxes)
	route("GET /v0/inboxes/{inbox}", core.PermInboxRead, s.getInbox)
	route("PATCH /v0/inboxes/{inbox}", core.PermInboxWrite, s.updateInbox)
	route("DELETE /v0/inboxes/{inbox}", core.PermInboxWrite, s.deleteInbox)
	route("GET /v0/inboxes/{inbox}/threads", core.PermMessageRead, s.listThreads)
	route("GET /v0/inboxes/{inbox}/threads/{thread}", core.PermMessageRead, s.getThread)
	route("GET /v0/threads", core.PermMessageRead, s.listThreads)
	route("GET /v0/threads/{thread}", core.PermMessageRead, s.getThread)
	route("GET /v0/inboxes/{inbox}/messages", core.PermMessageRead, s.listMessages)
	route("POST /v0/inboxes/{inbox}/messages/send", core.PermMessageSend, s.send)
	route("GET /v0/inboxes/{inbox}/messages/{message}", core.PermMessageRead, s.getMessage)
	route("PATCH /v0/inboxes/{inbox}/messages/{message}", core.PermMessageUpdate, s.updateMessage)
	route("GET /v0/inboxes/{inbox}/messages/{message}/raw", core.PermMessageRead, s.rawMessage)
	route("POST /v0/inboxes/{inbox}/messages/{message}/reply", core.PermMessageSend, s.reply)
	route("POST /v0/inboxes/{inbox}/messages/{message}/release", core.PermMessageRelease, s.releaseMessage)
	route("GET /v0/inboxes/{inbox}/messages/{message}/attachments/{attachment}", core.PermMessageRead, s.attachment)
	route("POST /v0/inboxes/{inbox}/drafts", core.PermMessageSend, s.createDraft)
	route("GET /v0/inboxes/{inbox}/drafts", core.PermMessageRead, s.listDrafts)
	route("GET /v0/inboxes/{inbox}/drafts/{draft}", core.PermMessageRead, s.getDraft)
	route("PATCH /v0/inboxes/{inbox}/drafts/{draft}", core.PermMessageSend, s.updateDraft)
	route("DELETE /v0/inboxes/{inbox}/drafts/{draft}", core.PermMessageSend, s.deleteDraft)
	route("POST /v0/inboxes/{inbox}/drafts/{draft}/send", core.PermMessageSend, s.sendDraft)
	route("GET /v0/events/stream", core.PermMessageRead, s.streamEvents)
	route("POST /v0/domains", core.PermDomainWrite, s.createDomain)
	route("GET /v0/domains", core.PermDomainRead, s.listDomains)
	route("GET /v0/domains/{domain}", core.PermDomainRead, s.getDomain)
	route("DELETE /v0/domains/{domain}", core.PermDomainWrite, s.deleteDomain)
	route("POST /v0/domains/{domain}/verify", core.PermDomainWrite, s.verifyDomain)
	route("POST /v0/webhooks", core.PermWebhookWrite, s.createWebhook)
	route("GET /v0/webhooks", core.PermWebhookRead, s.listWebhooks)
	route("DELETE /v0/webhooks/{webhook}", core.PermWebhookWrite, s.deleteWebhook)
	if s.MCP != nil {
		auth.Handle("/mcp", s.MCP)
	}
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openapi.Spec)
	})
	mux.Handle("/v0/", s.authenticate(auth))
	mux.Handle("/mcp", s.authenticate(auth))
	return s.gateway(mux)
}

// require checks a permission before the handler runs ("" means any key).
func (s *Server) require(perm string, h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if perm != "" {
			if err := PrincipalFrom(r.Context()).Require(perm); err != nil {
				s.fail(w, err)
				return
			}
		}
		h(w, r)
	})
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := apiKey(r)
		if key == "" {
			s.fail(w, &core.Error{Status: http.StatusUnauthorized, Name: "Unauthorized", Code: "missing_authorization",
				Message: "send Authorization: Bearer <api key>"})
			return
		}
		ip := clientIP(r.Context())
		if s.AuthFailures != nil && !s.AuthFailures.Peek(ip) {
			s.fail(w, &core.Error{Status: http.StatusTooManyRequests, Name: "Too Many Requests", Code: "rate_limit_exceeded",
				Message: "too many failed authentications from this address, retry later"})
			return
		}
		principal, err := s.Svc.Authenticate(r.Context(), key)
		if err != nil {
			if s.AuthFailures != nil && core.AsError(err).Status == http.StatusUnauthorized {
				s.AuthFailures.Allow(ip) // consume a token per failure
			}
			s.fail(w, err)
			return
		}
		org := principal.Org
		if s.Limiter != nil && !s.Limiter.Allow(org.ID) {
			s.fail(w, &core.Error{Status: http.StatusTooManyRequests, Name: "Too Many Requests", Code: "rate_limit_exceeded",
				Message: "too many requests for this organization, retry shortly"})
			return
		}
		setLogOrg(r.Context(), org.ID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, principal)))
	})
}

// apiKey reads the API key from Authorization: Bearer, or else X-Api-Key.
func apiKey(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return r.Header.Get("X-Api-Key")
}

func (s *Server) ok(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // Message-IDs like <id@host> stay readable
	if err := enc.Encode(v); err != nil {
		s.Log.Warn("write response", "err", err)
	}
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	e := core.AsError(err)
	if e.Status >= 500 {
		s.Log.Error("request failed", "err", err)
	}
	if e.Status == http.StatusTooManyRequests {
		retry := "60"
		if e.Code == "rate_limit_exceeded" {
			retry = "1"
		}
		w.Header().Set("Retry-After", retry)
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

// includeReview reads include_review=true: held for review messages are
// returned only when asked for and only to keys holding message_review.
func includeReview(r *http.Request) (bool, error) {
	v := r.URL.Query().Get("include_review")
	if v == "" {
		return false, nil
	}
	on, err := strconv.ParseBool(v)
	if err != nil {
		return false, core.Validation("include_review must be true or false")
	}
	return on, PrincipalFrom(r.Context()).ReviewAccess(on)
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
	p := PrincipalFrom(r.Context())
	perms := p.Permissions
	if perms == nil {
		perms = core.AllPermissions
	}
	s.ok(w, http.StatusOK, map[string]any{"organization_id": p.Org.ID, "name": p.Org.Name, "plan": p.Org.Plan,
		"screen_before_notify": p.Org.ScreenBeforeNotify, "api_key_id": p.KeyID, "permissions": perms, "expires_at": p.ExpiresAt})
}

// audit records a security relevant action; a failure is logged, never fatal.
func (s *Server) audit(r *http.Request, action, target string, meta map[string]any) {
	p := PrincipalFrom(r.Context())
	raw, _ := json.Marshal(meta)
	if meta == nil {
		raw = nil
	}
	err := s.Svc.Audit(context.WithoutCancel(r.Context()), p.Org.ID, core.AuditEntry{Actor: p.Actor(), Action: action,
		Target: target, IP: clientIP(r.Context()), RequestID: requestID(r.Context()), Metadata: raw})
	if err != nil {
		s.Log.Error("audit write failed", "action", action, "err", err)
	}
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	var req core.CreateKeyRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	k, err := s.Svc.CreateScopedKey(r.Context(), PrincipalFrom(r.Context()), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "api_key.create", k.ID, map[string]any{"name": k.Name, "permissions": k.Permissions})
	s.ok(w, http.StatusCreated, k)
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	items, err := s.Svc.ListKeys(r.Context(), PrincipalFrom(r.Context()))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("api_keys", items, ""))
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.RevokeKey(r.Context(), PrincipalFrom(r.Context()), r.PathValue("key")); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "api_key.revoke", r.PathValue("key"), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := s.Svc.ListAudit(r.Context(), PrincipalFrom(r.Context()), limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("entries", items, ""))
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
		s.audit(r, "inbox.create", inbox.InboxID, nil)
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
	s.audit(r, "inbox.delete", r.PathValue("inbox"), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listThreads(w http.ResponseWriter, r *http.Request) {
	review, err := includeReview(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	items, next, err := s.Svc.ListThreads(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), page(r), r.URL.Query().Get("label"), review)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("threads", items, next))
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request) {
	review, err := includeReview(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	t, err := s.Svc.GetThread(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("thread"), review)
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
	review, err := includeReview(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	items, next, err := s.Svc.ListMessages(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), page(r), r.URL.Query().Get("label"), review)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("messages", items, next))
}

func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	review, err := includeReview(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	m, err := s.Svc.GetMessage(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("message"), review)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, m)
}

func (s *Server) releaseMessage(w http.ResponseWriter, r *http.Request) {
	m, err := s.Svc.ReleaseMessage(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("message"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "message.release", m.ID, map[string]any{"inbox": m.InboxID, "message_id": m.MessageID, "gatekeeper": m.Gatekeeper})
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
	review, err := includeReview(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	raw, err := s.Svc.RawMessage(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("message"), review)
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
	s.audit(r, "webhook.create", hook.ID, map[string]any{"url": hook.URL, "event_types": hook.EventTypes})
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
	s.audit(r, "webhook.delete", r.PathValue("webhook"), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) attachment(w http.ResponseWriter, r *http.Request) {
	review, err := includeReview(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	info, data, err := s.Svc.GetAttachment(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("inbox"), r.PathValue("message"), r.PathValue("attachment"), review)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", info.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func (s *Server) createDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	d, err := s.Svc.CreateDomain(r.Context(), OrgFrom(r.Context()).ID, req.Domain)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "domain.create", d.Name, nil)
	s.ok(w, http.StatusCreated, d)
}

func (s *Server) listDomains(w http.ResponseWriter, r *http.Request) {
	items, err := s.Svc.ListDomains(r.Context(), OrgFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("domains", items, ""))
}

func (s *Server) getDomain(w http.ResponseWriter, r *http.Request) {
	d, err := s.Svc.GetDomain(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("domain"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, d)
}

func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.DeleteDomain(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("domain")); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "domain.delete", r.PathValue("domain"), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) verifyDomain(w http.ResponseWriter, r *http.Request) {
	d, err := s.Svc.VerifyDomain(r.Context(), OrgFrom(r.Context()).ID, r.PathValue("domain"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, d)
}
