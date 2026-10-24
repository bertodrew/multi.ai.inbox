package api

// Web push subscriptions. The console asks for notification permission, gets a
// subscription from the browser and posts it here; on message.received the
// platform wakes the device with a payload that carries the inbox address and a
// link, never mail content. The endpoint and the two keys are capabilities:
// they are accepted, stored and never returned or logged.
//
// Contract: web/docs/push-design.md, schema: openapi/openapi.yaml.
// Every route needs notification_write and every change is audited.

import (
	"net/http"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

func (s *Server) subscribePush(w http.ResponseWriter, r *http.Request) {
	var req core.PushSubscribeRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	sub, err := s.Svc.SubscribePush(r.Context(), PrincipalFrom(r.Context()), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	// The audit entry names the device, never the endpoint or the keys.
	s.audit(r, "push.subscribe", sub.ID, map[string]any{"device": sub.Device})
	s.ok(w, http.StatusCreated, sub)
}

func (s *Server) listPushSubscriptions(w http.ResponseWriter, r *http.Request) {
	items, err := s.Svc.ListPushSubscriptions(r.Context(), PrincipalFrom(r.Context()))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, listBody("push_subscriptions", items, ""))
}

// unsubscribePush accepts the id in the path, as the console uses it, and an
// endpoint in the body or the query, as the design allows for a proxy that
// strips DELETE bodies. A 204 is answered either way: the caller is dropping a
// device, and an endpoint this organization does not have is already off.
func (s *Server) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("subscription")
	endpoint := r.URL.Query().Get("endpoint")
	if id == "" && endpoint == "" && r.ContentLength != 0 {
		var req struct {
			Endpoint string `json:"endpoint"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, err)
			return
		}
		endpoint = req.Endpoint
	}
	if err := s.Svc.UnsubscribePush(r.Context(), PrincipalFrom(r.Context()), id, endpoint); err != nil {
		s.fail(w, err)
		return
	}
	target := id
	if target == "" {
		target = "endpoint" // never the endpoint itself
	}
	s.audit(r, "push.unsubscribe", target, nil)
	w.WriteHeader(http.StatusNoContent)
}
