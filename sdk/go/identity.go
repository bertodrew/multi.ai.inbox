package aimulti

import (
	"context"
	"net/http"
	"net/url"
)

// Me returns the authenticated caller: the organization, its plan, the
// permissions of the key and, for a session, the person.
func (c *Client) Me(ctx context.Context) (*Me, error) {
	out := &Me{}
	if err := c.getJSON(ctx, "/me", nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// RequestMagicLink emails a single use sign in link. It answers the same way
// for a known and an unknown address, so it never reveals who has an account.
func (c *Client) RequestMagicLink(ctx context.Context, email, redirectAfter string) error {
	body := struct {
		Email         string `json:"email"`
		RedirectAfter string `json:"redirect_after,omitempty"`
	}{email, redirectAfter}
	return c.sendJSON(ctx, http.MethodPost, "/auth/magic-link", body, nil, nil)
}

// RefreshSession rotates a single use refresh token into a new session. A
// replayed token answers 401 invalid_refresh_token.
func (c *Client) RefreshSession(ctx context.Context, refreshToken string) (*Session, error) {
	body := struct {
		RefreshToken string `json:"refresh_token"`
	}{refreshToken}
	out := &Session{}
	if err := c.sendJSON(ctx, http.MethodPost, "/auth/refresh", body, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Logout ends the session of the refresh token. It is idempotent: a token that
// is already invalid is not an error.
func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	body := struct {
		RefreshToken string `json:"refresh_token,omitempty"`
	}{refreshToken}
	return c.sendJSON(ctx, http.MethodPost, "/auth/logout", body, nil, nil)
}

// OAuthStart returns the Google or GitHub URL to send a browser to, with the
// state and PKCE challenge already set.
func (c *Client) OAuthStart(ctx context.Context, provider, redirectAfter string) (*OAuthStart, error) {
	q := url.Values{}
	if redirectAfter != "" {
		q.Set("redirect_after", redirectAfter)
	}
	out := &OAuthStart{}
	path := "/auth/oauth/" + escape(provider) + "/start"
	if err := c.getJSON(ctx, path, q, out); err != nil {
		return nil, err
	}
	return out, nil
}
