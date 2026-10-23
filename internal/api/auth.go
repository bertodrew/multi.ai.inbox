package api

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/bertodrew/multi.ai.inbox/internal/auth"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

// AuthSurface holds what the API needs to build links and redirects for
// people signing in. Tokens and providers live in core.AuthConfig.
type AuthSurface struct {
	// WebOrigin is the site that hosts /login, e.g. https://aimulti.inbox.
	WebOrigin string
	// PublicURL is the base of this API, used in magic links. When empty the
	// request's own scheme and host are used.
	PublicURL string
}

const maxAuthBody = 64 << 10

func (s *Server) web() string {
	return strings.TrimRight(s.Auth.WebOrigin, "/")
}

// base returns the public base URL of this API for links we email.
func (s *Server) base(r *http.Request) string {
	if s.Auth.PublicURL != "" {
		return strings.TrimRight(s.Auth.PublicURL, "/")
	}
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// wantsRedirect reports whether the caller is a browser that should end up on
// the site, instead of a client reading JSON.
func wantsRedirect(r *http.Request) bool {
	switch r.URL.Query().Get("format") {
	case "redirect":
		return true
	case "json":
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func isForm(r *http.Request) bool {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return ct == "application/x-www-form-urlencoded" || ct == "multipart/form-data"
}

// loginRedirect sends the browser back to /login with either a session in the
// fragment or an error code. The fragment never reaches a server or a log.
func (s *Server) loginRedirect(w http.ResponseWriter, r *http.Request, grant *core.SessionGrant, code string) {
	target := s.web() + "/login/"
	if grant == nil {
		q := url.Values{}
		q.Set("error", code)
		http.Redirect(w, r, target+"?"+q.Encode(), http.StatusSeeOther)
		return
	}
	q := url.Values{}
	q.Set("access_token", grant.AccessToken)
	q.Set("refresh_token", grant.RefreshToken)
	q.Set("token_type", grant.TokenType)
	q.Set("expires_in", strconv.Itoa(grant.ExpiresIn))
	http.Redirect(w, r, target+"#"+q.Encode(), http.StatusSeeOther)
}

// magicLink sends a single use sign in link. The answer never depends on
// whether the address has an account.
func (s *Server) magicLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBody)
	var req struct {
		Email         string `json:"email"`
		RedirectAfter string `json:"redirect_after"`
	}
	form := isForm(r)
	if form {
		if err := r.ParseForm(); err != nil {
			s.loginRedirect(w, r, nil, "invalid_request")
			return
		}
		req.Email = r.PostForm.Get("email")
		req.RedirectAfter = r.PostForm.Get("redirect_after")
	} else if err := decode(r, &req); err != nil {
		if wantsRedirect(r) {
			s.loginRedirect(w, r, nil, core.AsError(err).Code)
			return
		}
		s.fail(w, err)
		return
	}
	redirect := form || wantsRedirect(r)
	if s.MagicLinks != nil && !s.MagicLinks.Allow(clientIP(r.Context())) {
		if redirect {
			s.loginRedirect(w, r, nil, "rate_limit_exceeded")
			return
		}
		s.fail(w, &core.Error{Status: http.StatusTooManyRequests, Name: "Too Many Requests", Code: "rate_limit_exceeded",
			Message: "too many sign in links requested from this address, retry in a few minutes"})
		return
	}
	if err := s.Svc.RequestMagicLink(r.Context(), req.Email, s.base(r), req.RedirectAfter, clientIP(r.Context())); err != nil {
		if redirect {
			s.loginRedirect(w, r, nil, core.AsError(err).Code)
			return
		}
		s.fail(w, err)
		return
	}
	if redirect {
		target := s.web() + "/login/"
		if after := core.SafeRedirect(req.RedirectAfter); after != "" {
			target = s.web() + after
		}
		http.Redirect(w, r, withQuery(target, "sent", "1"), http.StatusSeeOther)
		return
	}
	expires := 0
	if s.Svc.Auth != nil {
		expires = s.Svc.Auth.MagicTTLSeconds()
	}
	s.ok(w, http.StatusAccepted, map[string]any{"status": "sent", "expires_in": expires})
}

func withQuery(target, key, value string) string {
	u, err := url.Parse(target)
	if err != nil {
		return target
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

// authCallback finishes a sign in: a magic link token, or an OAuth code with
// its state. A browser is sent back to /login with the session in the
// fragment; a client asking for JSON gets the session in the body.
func (s *Server) authCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect := wantsRedirect(r)
	var (
		grant *core.SessionGrant
		err   error
	)
	switch {
	case q.Get("token") != "":
		grant, err = s.Svc.CompleteMagicLink(r.Context(), q.Get("token"), clientIP(r.Context()), r.UserAgent())
	case q.Get("error") != "":
		// The person refused, or the provider refused: no account change.
		if redirect {
			s.loginRedirect(w, r, nil, "access_denied")
			return
		}
		s.fail(w, &core.Error{Status: http.StatusBadRequest, Name: "Bad Request", Code: "access_denied",
			Message: "the identity provider did not complete the sign in"})
		return
	case q.Get("code") != "" || q.Get("state") != "":
		grant, err = s.Svc.CompleteOAuth(r.Context(), q.Get("code"), q.Get("state"), clientIP(r.Context()), r.UserAgent())
	default:
		err = core.Validation("send token, or code and state")
	}
	if err != nil {
		if redirect {
			s.loginRedirect(w, r, nil, core.AsError(err).Code)
			return
		}
		s.fail(w, err)
		return
	}
	s.auditSignIn(r, grant)
	if redirect {
		s.loginRedirect(w, r, grant, "")
		return
	}
	s.ok(w, http.StatusOK, grant)
}

// auditSignIn records who signed in, without the tokens.
func (s *Server) auditSignIn(r *http.Request, grant *core.SessionGrant) {
	if grant == nil || grant.User == nil || grant.Organization == nil {
		return
	}
	raw, _ := json.Marshal(map[string]any{"provider": grant.User.Provider, "role": grant.Role, "email": grant.User.Email})
	err := s.Svc.Audit(r.Context(), grant.Organization.ID, core.AuditEntry{Actor: "user:" + grant.User.ID,
		Action: "user.sign_in", Target: grant.User.ID, IP: clientIP(r.Context()), RequestID: requestID(r.Context()), Metadata: raw})
	if err != nil {
		s.Log.Error("audit write failed", "action", "user.sign_in", "err", err)
	}
}

// oauthStart begins the authorization code flow with PKCE. A browser follows
// the redirect; a client gets the URL and the state as JSON.
func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	start, err := s.Svc.StartOAuth(r.Context(), r.PathValue("provider"), s.base(r)+"/v0/auth/callback",
		r.URL.Query().Get("redirect_after"))
	if err != nil {
		if wantsRedirect(r) {
			s.loginRedirect(w, r, nil, core.AsError(err).Code)
			return
		}
		s.fail(w, err)
		return
	}
	if wantsRedirect(r) || r.URL.Query().Get("redirect") == "1" {
		http.Redirect(w, r, start.AuthorizeURL, http.StatusFound)
		return
	}
	s.ok(w, http.StatusOK, start)
}

// refreshSession rotates a refresh token into a new session.
func (s *Server) refreshSession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBody)
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	grant, err := s.Svc.Refresh(r.Context(), req.RefreshToken, clientIP(r.Context()), r.UserAgent())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.ok(w, http.StatusOK, grant)
}

// logout revokes the session behind the refresh token in the body, the access
// token in the Authorization header, or both.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBody)
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, err)
			return
		}
	}
	access, fromHeader := bearer(r)
	if !fromHeader || !auth.LooksLikeJWT(access) {
		access = "" // an API key is revoked through its own endpoint
	}
	if err := s.Svc.Logout(r.Context(), req.RefreshToken, access); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// me answers GET /v0/me: the person and organization behind a session, or the
// key and organization behind an API key.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	perms := p.Permissions
	if perms == nil {
		// A full access key holds every permission except the explicit ones.
		perms = slices.DeleteFunc(slices.Clone(core.AllPermissions), func(perm string) bool {
			return slices.Contains(core.ExplicitPermissions, perm)
		})
	}
	body := map[string]any{"auth": "api_key", "organization_id": p.Org.ID, "name": p.Org.Name, "plan": p.Org.Plan,
		"screen_before_notify": p.Org.ScreenBeforeNotify, "api_key_id": p.KeyID, "permissions": perms, "expires_at": p.ExpiresAt}
	if p.UserID != "" {
		body["auth"] = "session"
		body["api_key_id"] = nil
		body["role"] = p.Role
		body["user"] = map[string]any{"user_id": p.UserID, "email": p.UserEmail}
		body["organization"] = p.Org
	}
	s.ok(w, http.StatusOK, body)
}

// bearer reads a credential from Authorization: Bearer or X-Api-Key.
func bearer(r *http.Request) (string, bool) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")), true
	}
	return r.Header.Get("X-Api-Key"), false
}
