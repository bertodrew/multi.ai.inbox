package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	authpkg "github.com/bertodrew/multi.ai.inbox/internal/auth"
	"github.com/bertodrew/multi.ai.inbox/internal/blob"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

// anon sends a request without a credential: the sign in endpoints are the
// only /v0/ routes that answer without one.
func anon(t *testing.T, base, method, path, body string, headers map[string]string) (int, map[string]any, http.Header) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, resp.Header
}

var linkToken = regexp.MustCompile(`token=([A-Za-z0-9_-]{20,})`)

// signInToken decodes the text part of the captured sign in email and returns
// the single use token in its link.
func signInToken(t *testing.T, raw []byte) string {
	t.Helper()
	_, body, ok := strings.Cut(string(raw), "\r\n\r\n")
	if !ok {
		t.Fatalf("email has no body:\n%s", raw)
	}
	text, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", ""))
	if err != nil {
		t.Fatalf("email body is not base64: %v", err)
	}
	m := linkToken.FindStringSubmatch(string(text))
	if m == nil {
		t.Fatalf("no sign in token in the email:\n%s", text)
	}
	return m[1]
}

func TestUserSignIn(t *testing.T) {
	ctx := context.Background()
	d := openPlatform(t, ctx, freshDatabase(t))
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	// A local relay stands in for MAI_SMTP_RELAY: no external mail is sent.
	relay := &capture{}
	relaySrv := smtp.NewServer(relay)
	relaySrv.Domain = "relay.test"
	relaySrv.AllowInsecureAuth = true
	relayAddr := listen(t, relaySrv)

	from := "no-reply@" + shared
	svc := &core.Service{DB: d, Blob: blob.FS{Root: t.TempDir()}, SharedDomain: shared, Hostname: "mx." + shared,
		Auth: &core.AuthConfig{Secret: bytes.Repeat([]byte("k"), 32), WebOrigin: "https://site.test", MailFrom: from,
			Mailer: &authpkg.RelayMailer{Relay: "smtp+insecure://" + relayAddr, From: from, Helo: "mx." + shared, Timeout: 5 * time.Second}}}
	srv := &api.Server{Svc: svc, Log: log, Auth: api.AuthSurface{WebOrigin: "https://site.test", PublicURL: "https://api.test"}}
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	// Magic link: the answer is the same for any address, and the mail carries
	// the only copy of the token.
	if code, body, _ := anon(t, httpSrv.URL, "POST", "/v0/auth/magic-link", `{"email":"Ada@Example.com"}`, nil); code != 202 {
		t.Fatalf("magic link: %d %v", code, body)
	}
	relay.mu.Lock()
	if len(relay.msgs) != 1 {
		relay.mu.Unlock()
		t.Fatalf("relay got %d messages, want 1", len(relay.msgs))
	}
	msg := relay.msgs[0]
	relay.mu.Unlock()
	if len(msg.to) != 1 || msg.to[0] != "ada@example.com" {
		t.Fatalf("envelope to = %v, want the normalized address", msg.to)
	}
	token := signInToken(t, msg.raw)

	code, grant, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/callback?token="+url.QueryEscape(token), "",
		map[string]string{"Accept": "application/json"})
	if code != 200 {
		t.Fatalf("callback: %d %v", code, grant)
	}
	access, _ := grant["access_token"].(string)
	refresh, _ := grant["refresh_token"].(string)
	if access == "" || refresh == "" || grant["role"] != "owner" || grant["token_type"] != "Bearer" {
		t.Fatalf("grant = %v", grant)
	}
	org, _ := grant["organization"].(map[string]any)
	orgID, _ := org["organization_id"].(string)
	user, _ := grant["user"].(map[string]any)
	if orgID == "" || user["email"] != "ada@example.com" {
		t.Fatalf("grant = %v", grant)
	}

	// The token works once.
	if code, body, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/callback?token="+url.QueryEscape(token), "",
		map[string]string{"Accept": "application/json"}); code != 400 || body["code"] != "invalid_magic_link" {
		t.Fatalf("token replay: %d %v", code, body)
	}

	// The session token opens the API, with the owner's permissions.
	c := client{t: t, base: httpSrv.URL, key: access}
	code, me := c.do("GET", "/v0/me", nil, nil)
	if code != 200 || me["auth"] != "session" || me["role"] != "owner" || me["organization_id"] != orgID {
		t.Fatalf("me with a session: %d %v", code, me)
	}
	if code, body := c.do("GET", "/v0/inboxes", nil, nil); code != 200 {
		t.Fatalf("list inboxes with a session: %d %v", code, body)
	}

	// Rotation: the old refresh token dies with the exchange.
	code, rotated, _ := anon(t, httpSrv.URL, "POST", "/v0/auth/refresh", `{"refresh_token":"`+refresh+`"}`, nil)
	if code != 200 {
		t.Fatalf("refresh: %d %v", code, rotated)
	}
	nextRefresh, _ := rotated["refresh_token"].(string)
	if nextRefresh == "" || nextRefresh == refresh {
		t.Fatalf("refresh token was not rotated: %v", rotated)
	}
	if code, body, _ := anon(t, httpSrv.URL, "POST", "/v0/auth/refresh", `{"refresh_token":"`+refresh+`"}`, nil); code != 401 {
		t.Fatalf("replayed refresh token: %d %v", code, body)
	}

	// Logout ends the access token immediately, not in fifteen minutes.
	nextAccess, _ := rotated["access_token"].(string)
	after := client{t: t, base: httpSrv.URL, key: nextAccess}
	if code, body := after.do("POST", "/v0/auth/logout", map[string]any{}, nil); code != 204 {
		t.Fatalf("logout: %d %v", code, body)
	}
	if code, body := after.do("GET", "/v0/me", nil, nil); code != 401 || body["code"] != "session_revoked" {
		t.Fatalf("me after logout: %d %v", code, body)
	}

	// An API key for the same organization keeps working: agents are not
	// affected by a person signing out.
	key, err := svc.CreateKey(ctx, orgID, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if code, body := (client{t: t, base: httpSrv.URL, key: key.Key}).do("GET", "/v0/inboxes", nil, nil); code != 200 {
		t.Fatalf("agent key after logout: %d %v", code, body)
	}

	t.Run("sign in again keeps one account", func(t *testing.T) {
		if code, body, _ := anon(t, httpSrv.URL, "POST", "/v0/auth/magic-link", `{"email":"ada@example.com"}`, nil); code != 202 {
			t.Fatalf("magic link: %d %v", code, body)
		}
		relay.mu.Lock()
		if len(relay.msgs) != 2 {
			relay.mu.Unlock()
			t.Fatalf("relay got %d messages, want 2", len(relay.msgs))
		}
		second := relay.msgs[1]
		relay.mu.Unlock()
		code, again, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/callback?token="+url.QueryEscape(signInToken(t, second.raw)), "",
			map[string]string{"Accept": "application/json"})
		if code != 200 {
			t.Fatalf("second callback: %d %v", code, again)
		}
		u, _ := again["user"].(map[string]any)
		o, _ := again["organization"].(map[string]any)
		if u["user_id"] != user["user_id"] || o["organization_id"] != orgID {
			t.Fatalf("a second sign in created a new account: %v", again)
		}
		if again["role"] != "owner" {
			t.Fatalf("role = %v", again["role"])
		}
	})

	// People are platform level records: the tenant role cannot read or write
	// them, not even inside its own organization.
	var visible int
	if err := d.Pool.QueryRow(ctx, "select count(*) from users").Scan(&visible); err != nil {
		t.Fatalf("mai_app reading users: %v", err)
	}
	if visible != 0 {
		t.Fatalf("mai_app sees %d users", visible)
	}
	if err := d.Pool.QueryRow(ctx, "select count(*) from sessions").Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("mai_app sees %d sessions (%v)", visible, err)
	}
	if _, err := d.Pool.Exec(ctx, "insert into users (email) values ('forged@example.com')"); err == nil {
		t.Fatal("mai_app inserted a user")
	}
	// A tenant may see its members but a role change is a system decision:
	// the update matches no row without the system context.
	err = d.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "update memberships set role = 'member'")
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			t.Fatalf("mai_app changed %d memberships", tag.RowsAffected())
		}
		var role string
		if err := tx.QueryRow(ctx, "select role from memberships").Scan(&role); err != nil {
			return err
		}
		if role != core.RoleOwner {
			t.Fatalf("role became %q", role)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A link past its fifteen minutes is refused, exactly like a used one.
	t.Run("expired link refused", func(t *testing.T) {
		late, err := authpkg.NewToken(32)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.WithSystem(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `insert into magic_link_tokens (token_hash, email, expires_at)
				values ($1, $2, now() - interval '1 minute')`, authpkg.HashToken(late), "late@example.com")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		code, body, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/callback?token="+url.QueryEscape(late), "",
			map[string]string{"Accept": "application/json"})
		if code != 400 || body["code"] != "invalid_magic_link" {
			t.Fatalf("expired link: %d %v", code, body)
		}
	})
}

// OAuthStart is checked against a local provider: the verifier the API stored
// must hash to the challenge it sent, so PKCE is proven end to end.
func TestOAuthSignIn(t *testing.T) {
	ctx := context.Background()
	d := openPlatform(t, ctx, freshDatabase(t))
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	var mu sync.Mutex
	var wantChallenge string
	verified := true
	profile := `{"sub":"google-42","email":"person@example.com","email_verified":%v,"name":"Ada","picture":"https://img/1"}`
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		challenge := wantChallenge
		mu.Unlock()
		if r.Form.Get("code") != "good-code" || authpkg.ChallengeS256(r.Form.Get("code_verifier")) != challenge {
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"bearer"}`))
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ok := verified
		mu.Unlock()
		_, _ = fmt.Fprintf(w, profile, ok)
	})
	provider := httptest.NewServer(mux)
	defer provider.Close()

	svc := &core.Service{DB: d, Blob: blob.FS{Root: t.TempDir()}, SharedDomain: shared, Hostname: "mx." + shared,
		Auth: &core.AuthConfig{Secret: bytes.Repeat([]byte("k"), 32), WebOrigin: "https://site.test",
			Google: authpkg.Provider{Name: "google", ClientID: "id", ClientSecret: "secret",
				AuthURL: provider.URL + "/authorize", TokenURL: provider.URL + "/token", UserURL: provider.URL + "/user",
				Scopes: []string{"openid", "email", "profile"}}}}
	srv := &api.Server{Svc: svc, Log: log, Auth: api.AuthSurface{WebOrigin: "https://site.test", PublicURL: "https://api.test"}}
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	code, start, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/oauth/google/start", "", nil)
	if code != 200 {
		t.Fatalf("start: %d %v", code, start)
	}
	authURL, _ := start["authorize_url"].(string)
	state, _ := start["state"].(string)
	u, err := url.Parse(authURL)
	if err != nil || state == "" {
		t.Fatalf("authorize_url = %q (%v)", authURL, err)
	}
	if u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") == "" ||
		u.Query().Get("redirect_uri") != "https://api.test/v0/auth/callback" {
		t.Fatalf("authorize_url lacks PKCE: %s", authURL)
	}
	mu.Lock()
	wantChallenge = u.Query().Get("code_challenge")
	mu.Unlock()

	code, grant, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/callback?code=good-code&state="+url.QueryEscape(state), "",
		map[string]string{"Accept": "application/json"})
	if code != 200 || grant["role"] != "owner" {
		t.Fatalf("callback: %d %v", code, grant)
	}
	user, _ := grant["user"].(map[string]any)
	if user["email"] != "person@example.com" || user["provider"] != "google" {
		t.Fatalf("user = %v", user)
	}

	// The state works once.
	if code, body, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/callback?code=good-code&state="+url.QueryEscape(state), "",
		map[string]string{"Accept": "application/json"}); code != 400 || body["code"] != "invalid_state" {
		t.Fatalf("state replay: %d %v", code, body)
	}

	// A provider that does not vouch for the address cannot sign anyone in.
	mu.Lock()
	verified = false
	mu.Unlock()
	_, start2, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/oauth/google/start", "", nil)
	authURL2, _ := start2["authorize_url"].(string)
	state2, _ := start2["state"].(string)
	u2, _ := url.Parse(authURL2)
	mu.Lock()
	wantChallenge = u2.Query().Get("code_challenge")
	mu.Unlock()
	if code, body, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/callback?code=good-code&state="+url.QueryEscape(state2), "",
		map[string]string{"Accept": "application/json"}); code != 403 || body["code"] != "email_not_verified" {
		t.Fatalf("unverified email: %d %v", code, body)
	}

	// A browser is sent to /login with the session in the fragment.
	mu.Lock()
	verified = true
	mu.Unlock()
	_, start3, _ := anon(t, httpSrv.URL, "GET", "/v0/auth/oauth/google/start", "", nil)
	authURL3, _ := start3["authorize_url"].(string)
	state3, _ := start3["state"].(string)
	u3, _ := url.Parse(authURL3)
	mu.Lock()
	wantChallenge = u3.Query().Get("code_challenge")
	mu.Unlock()
	req, _ := http.NewRequest("GET", httpSrv.URL+"/v0/auth/callback?code=good-code&state="+url.QueryEscape(state3), nil)
	req.Header.Set("Accept", "text/html")
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://site.test/login/#") || !strings.Contains(loc, "access_token=") {
		t.Fatalf("browser redirect = %q", loc)
	}
}
