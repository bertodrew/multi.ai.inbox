package main

// Account auth: real sign-up (first name, last name, country, password +
// confirm) and the OAuth entry points. The account store is file-backed JSON
// in LANDING_CRED_DIR; passwords are never stored in clear text — sha256 of a
// per-account random salt + password. Honest limit: production must migrate
// this to argon2id/bcrypt before launch; the salted hash matches the demo
// store grade used elsewhere in this app.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type account struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Country   string `json:"country"`
	Salt      string `json:"salt"` // random, per account
	Hash      string `json:"hash"` // sha256(salt + ":" + password), hex
	CreatedAt string `json:"created_at"`
}

func accountPath(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(email)))
	return filepath.Join(credDir(), "account_"+hex.EncodeToString(sum[:12])+".json")
}

func writeAccount(a *account) error {
	if err := os.MkdirAll(credDir(), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	tmp := filepath.Join(credDir(), "."+filepath.Base(accountPath(a.Email))+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, accountPath(a.Email))
}

func readAccount(email string) (*account, error) {
	b, err := os.ReadFile(accountPath(email))
	if err != nil {
		return nil, err
	}
	var a account
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func hashPassword(salt, password string) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(sum[:])
}

func randomSalt() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return newToken(16)
	}
	return hex.EncodeToString(b)
}

// handleRegister serves POST /api/auth/register (form: first_name, last_name,
// email, country, password, password_confirm + CSRF). Every field is
// validated, the account is stored hashed, the session rotates and the user
// is signed in — same identity flow as a connect.
func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	sid, sess := ensureSession(w, r)
	_ = sid
	if !requireCSRF(w, r, sess) {
		return
	}
	first := strings.TrimSpace(r.PostFormValue("first_name"))
	last := strings.TrimSpace(r.PostFormValue("last_name"))
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	country := strings.TrimSpace(r.PostFormValue("country"))
	password := r.PostFormValue("password")
	confirm := r.PostFormValue("password_confirm")

	switch {
	case first == "" || last == "":
		jsonErr(w, "First and last name are required", http.StatusBadRequest)
		return
	case email == "" || !emailRe.MatchString(email):
		jsonErr(w, "A valid email address is required", http.StatusBadRequest)
		return
	case country == "":
		jsonErr(w, "Country is required", http.StatusBadRequest)
		return
	case len(password) < 8:
		jsonErr(w, "Password must be at least 8 characters long", http.StatusBadRequest)
		return
	case password != confirm:
		jsonErr(w, "Passwords do not match", http.StatusBadRequest)
		return
	}
	if _, err := readAccount(email); err == nil {
		jsonErr(w, "An account with this email already exists — sign in instead", http.StatusConflict)
		return
	}
	a := &account{
		FirstName: first,
		LastName:  last,
		Email:     email,
		Country:   country,
		Salt:      randomSalt(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	a.Hash = hashPassword(a.Salt, password)
	if err := writeAccount(a); err != nil {
		jsonErr(w, "Account storage unavailable", http.StatusInternalServerError)
		return
	}
	// Rotate FIRST (fresh identity on a successful sign-up), then sign in —
	// same order as the connect path.
	newSID := rotateSession(w, r, sess)
	sessions.mu.Lock()
	sessions.m[newSID].connected = true
	sessions.m[newSID].username = email
	sessions.m[newSID].expires = time.Now().Add(sessionTTL)
	sessions.mu.Unlock()
	auditLog("account_register", map[string]any{"ok": true, "username": email, "country": country})
	jsonOut(w, map[string]any{"ok": true, "username": email, "next": "onboarding.html"})
}

// handleOAuth serves GET /api/auth/oauth/{provider}. Fail-closed: without the
// provider's client id/secret in the env the button answers the honest
// not-configured error — no fake redirects.
func handleOAuth(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	switch provider {
	case "google", "github", "microsoft":
	default:
		jsonErr(w, "Unknown sign-in provider", http.StatusNotFound)
		return
	}
	prefix := strings.ToUpper(provider)
	id := os.Getenv(prefix + "_CLIENT_ID")
	secret := os.Getenv(prefix + "_CLIENT_SECRET")
	if id == "" || secret == "" {
		jsonErr(w, prefix+" sign-in is not configured yet — set "+prefix+"_CLIENT_ID and "+prefix+"_CLIENT_SECRET", http.StatusNotImplemented)
		return
	}
	origin := env("LANDING_BASE_URL", "https://"+r.Host)
	redirect := origin + "/landingpage/api/auth/oauth/" + provider + "/callback"
	var authorize string
	switch provider {
	case "google":
		authorize = "https://accounts.google.com/o/oauth2/v2/auth"
	case "github":
		authorize = "https://github.com/login/oauth/authorize"
	case "microsoft":
		authorize = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	}
	state := newToken(16)
	_, sess := ensureSession(w, r)
	sessions.mu.Lock()
	sess.oauthState = state
	sessions.mu.Unlock()
	q := url.Values{}
	q.Set("client_id", id)
	q.Set("redirect_uri", redirect)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	http.Redirect(w, r, authorize+"?"+q.Encode(), http.StatusFound)
}

// handleOAuthCallback serves GET /api/auth/oauth/{provider}/callback. The
// code exchange is not wired yet — the honest answer until then.
func handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	jsonErr(w, "OAuth callback not wired yet — complete the provider setup first", http.StatusNotImplemented)
}
