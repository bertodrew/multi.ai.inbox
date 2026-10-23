package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Provider describes one OAuth 2.0 authorization code provider. Every request
// uses PKCE with S256, so a stolen code is useless without the verifier.
type Provider struct {
	Name         string // "google" or "github"
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserURL      string
	// EmailsURL is GitHub's verified email list: /user hides the address when
	// the account keeps it private.
	EmailsURL string
	Scopes    []string
}

var (
	// ErrProviderUnavailable means the provider answered with an error or with
	// something we could not parse: the user did not do anything wrong.
	ErrProviderUnavailable = errors.New("auth: identity provider unavailable")
	// ErrCodeRejected means the provider refused the code or the verifier.
	ErrCodeRejected = errors.New("auth: authorization code rejected")
	// ErrEmailUnverified means the provider did not vouch for the address.
	ErrEmailUnverified = errors.New("auth: provider did not verify the email address")
)

// Google returns the Google provider (OIDC userinfo endpoint).
func Google(clientID, clientSecret string) Provider {
	return Provider{Name: "google", ClientID: clientID, ClientSecret: clientSecret,
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		UserURL:  "https://openidconnect.googleapis.com/v1/userinfo",
		Scopes:   []string{"openid", "email", "profile"}}
}

// GitHub returns the GitHub provider (REST API v3).
func GitHub(clientID, clientSecret string) Provider {
	return Provider{Name: "github", ClientID: clientID, ClientSecret: clientSecret,
		AuthURL:   "https://github.com/login/oauth/authorize",
		TokenURL:  "https://github.com/login/oauth/access_token",
		UserURL:   "https://api.github.com/user",
		EmailsURL: "https://api.github.com/user/emails",
		Scopes:    []string{"read:user", "user:email"}}
}

// Configured reports whether the provider has credentials. A provider without
// them answers 501 at the start endpoint instead of redirecting nowhere.
func (p Provider) Configured() bool {
	return p.ClientID != "" && p.ClientSecret != ""
}

// AuthorizeURL builds the redirect that starts the flow. state binds the
// callback to this browser, the challenge proves the code came back to us.
func (p Provider) AuthorizeURL(state, challenge, redirectURI string) string {
	q := url.Values{}
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(p.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	sep := "?"
	if strings.Contains(p.AuthURL, "?") {
		sep = "&"
	}
	return p.AuthURL + sep + q.Encode()
}

// Identity is what a provider vouches for. ProviderID is stable across email
// changes, so it is the key a user row is matched on.
type Identity struct {
	Provider      string
	ProviderID    string
	Email         string
	EmailVerified bool
	Name          string
	AvatarURL     string
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// Code exchanges an authorization code (with its PKCE verifier) for the
// identity behind it.
func (p Provider) Code(ctx context.Context, client *http.Client, code, verifier, redirectURI string) (*Identity, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("%w: unreadable token response", ErrProviderUnavailable)
	}
	if tok.Error != "" || tok.AccessToken == "" {
		// invalid_grant covers a wrong verifier, an expired code and a replay.
		if tok.Error == "invalid_grant" || tok.Error == "bad_verification_code" || tok.Error == "invalid_request" {
			return nil, ErrCodeRejected
		}
		if res.StatusCode >= 400 && res.StatusCode < 500 {
			return nil, ErrCodeRejected
		}
		return nil, fmt.Errorf("%w: %s", ErrProviderUnavailable, tok.Error)
	}
	return p.identity(ctx, client, tok.AccessToken)
}

func (p Provider) get(ctx context.Context, client *http.Client, at, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	req.Header.Set("Authorization", "Bearer "+at)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "multi.ai.inbox")
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrProviderUnavailable, res.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: unreadable profile response", ErrProviderUnavailable)
	}
	return nil
}

func (p Provider) identity(ctx context.Context, client *http.Client, at string) (*Identity, error) {
	switch p.Name {
	case "google":
		var u struct {
			Sub           string `json:"sub"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
			Picture       string `json:"picture"`
		}
		if err := p.get(ctx, client, at, p.UserURL, &u); err != nil {
			return nil, err
		}
		if u.Sub == "" || u.Email == "" {
			return nil, fmt.Errorf("%w: profile without subject or email", ErrProviderUnavailable)
		}
		return &Identity{Provider: p.Name, ProviderID: u.Sub, Email: u.Email,
			EmailVerified: u.EmailVerified, Name: u.Name, AvatarURL: u.Picture}, nil
	case "github":
		var u struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := p.get(ctx, client, at, p.UserURL, &u); err != nil {
			return nil, err
		}
		if u.ID == 0 {
			return nil, fmt.Errorf("%w: profile without id", ErrProviderUnavailable)
		}
		id := &Identity{Provider: p.Name, ProviderID: strconv.FormatInt(u.ID, 10), Email: u.Email,
			Name: u.Name, AvatarURL: u.AvatarURL}
		if id.Name == "" {
			id.Name = u.Login
		}
		// The address on /user is a public profile field that GitHub does not
		// promise to verify, so the verified list decides. A private address
		// only appears there too.
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if p.EmailsURL == "" {
			return nil, ErrEmailUnverified
		}
		if err := p.get(ctx, client, at, p.EmailsURL, &emails); err != nil {
			return nil, err
		}
		// Only the primary address signs in: picking a secondary one would
		// silently create an account under an address the person did not
		// choose on the provider.
		for _, e := range emails {
			if e.Primary && e.Verified {
				id.Email, id.EmailVerified = e.Email, true
				return id, nil
			}
		}
		return nil, ErrEmailUnverified
	default:
		return nil, fmt.Errorf("%w: unknown provider %q", ErrProviderUnavailable, p.Name)
	}
}
