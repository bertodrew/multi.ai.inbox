// multi.ai.inbox landing API — Go rewrite of the PHP landing/php (bootstrap, connect,
// disconnect, status, domains, dns, namecheap, mcp). Single binary, stdlib only.
//
// Security model (fixes the 4 Strix findings on the PHP version):
//   - S1 session fixation (CVSS 9.6): sessions are server-issued 32-byte random tokens;
//     client-supplied cookies are never trusted as identifiers, and the token is rotated
//     (cookie re-issued, old store invalidated) on every successful connect.
//   - S2 global credential file (CVSS 9.9): credentials are stored PER SESSION in
//     namecheap_<sha256(sid)[:16]>.json, AES-256-GCM encrypted at rest with a master key
//     (AAD binds the record to the session id), so two users never share state.
//   - S3 brute-force per session only (CVSS 5.3): failed connects are limited per IP
//     (5 / 10 min) in addition to per session.
//   - S4 status oracle (CVSS 5.3): /api/status no longer discloses the server egress IP;
//     the connect flow takes the client IP from the connection itself (X-Real-IP/RemoteAddr).
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

const (
	cookieName     = "MULTIAI_LANDING"
	maxAttempts    = 5
	attemptWindow  = 10 * time.Minute
	credTTL        = 24 * time.Hour // orphaned per-session credential files are reaped after this
	internalAddr   = ":8080"
	ncPageSize     = 100
	ncMaxPages     = 10
	ncConnectTO    = 10 * time.Second
	ncTotalTO      = 25 * time.Second
	mcConnectTO    = 5 * time.Second
	mcTotalTO      = 20 * time.Second
	mcpProtoVer    = "2025-06-18"
	protocolOrigin = "landing-go"
)

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?\.)+([a-z]{2,63}|xn--[a-z0-9-]{4,59})$`)

// validDomain mirrors the PHP check: full-length <= 253 + per-label grammar
// (Go/RE2 has no lookaheads, so the length guard lives here).
func validDomain(d string) bool {
	return len(d) >= 1 && len(d) <= 253 && domainRe.MatchString(d)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func credDir() string { return env("LANDING_CRED_DIR", "/credentials") }

func ncAPIURL(sandbox bool) string {
	key, def := "NAMECHEAP_API_URL", "https://api.namecheap.com/xml.response"
	if sandbox {
		key, def = "NAMECHEAP_SANDBOX_API_URL", "https://api.sandbox.namecheap.com/xml.response"
	}
	return env(key, def)
}

// ---------------------------------------------------------------------------
// json helpers
// ---------------------------------------------------------------------------

func jsonOut(w http.ResponseWriter, data map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Landing-Runtime", protocolOrigin)
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(data)
}

func jsonErr(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"ok": false, "error": message})
}

// ---------------------------------------------------------------------------
// sessions (S1: server-issued tokens, rotated on connect)
// ---------------------------------------------------------------------------

type session struct {
	connected bool
	username  string
	attempts  []time.Time
	csrf      string
	expires   time.Time // zero = anonymous session (no connection to expire)
}

// sessionTTL: a connected session dies one working day after the connect —
// stale "connected" cookies never survive overnight (auto-login bug, 20/10).
const sessionTTL = 24 * time.Hour

type sessionStore struct {
	mu sync.Mutex
	m  map[string]*session
}

var sessions = &sessionStore{m: map[string]*session{}}

func newToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand unavailable: %v", err)
	}
	return hex.EncodeToString(b)
}

// getSession returns the live session for the request cookie. A client-supplied
// value that does not exist in the server-side store is treated as anonymous —
// the identifier is never trusted, only matched (S1).
func getSession(r *http.Request) (sid string, sess *session) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return "", nil
	}
	sessions.mu.Lock()
	s, ok := sessions.m[c.Value]
	expired := ok && s.connected && !s.expires.IsZero() && time.Now().After(s.expires)
	if expired {
		delete(sessions.m, c.Value)
	}
	sessions.mu.Unlock()
	if expired {
		// A connected session past its TTL is anonymous again: drop the record
		// and its credential file — the cookie identifier is never trusted, only
		// matched against the live store (S1).
		removeCredFile(c.Value)
		return "", nil
	}
	if ok {
		return c.Value, s
	}
	return "", nil
}

// ensureSession issues a fresh server-side session + cookie when the request
// has none (S1: identifiers are minted here, never read from the client).
func ensureSession(w http.ResponseWriter, r *http.Request) (sid string, sess *session) {
	if sid, sess = getSession(r); sess != nil {
		return sid, sess
	}
	sid = newToken(32)
	sess = &session{}
	sessions.mu.Lock()
	sessions.m[sid] = sess
	sessions.mu.Unlock()
	setCookie(w, sid)
	return sid, sess
}

func setCookie(w http.ResponseWriter, sid string) {
	secure := (os.Getenv("HTTPS_INFER") != "off") // behind the mailcow nginx the origin is https
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sid,
		Path:     "/landingpage",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// rotateSession re-issues the identity on a successful connect (S1): new sid,
// state moved over, old session + old credential file invalidated.
func rotateSession(w http.ResponseWriter, r *http.Request, sess *session) string {
	oldSID, _ := getSession(r)
	newSID := newToken(32)
	sessions.mu.Lock()
	sessions.m[newSID] = &session{connected: sess.connected, username: sess.username}
	sessions.mu.Unlock()
	if oldSID != "" {
		sessions.mu.Lock()
		delete(sessions.m, oldSID)
		sessions.mu.Unlock()
		removeCredFile(oldSID)
	}
	setCookie(w, newSID)
	return newSID
}

func dropSession(w http.ResponseWriter, r *http.Request) {
	if sid, _ := getSession(r); sid != "" {
		sessions.mu.Lock()
		delete(sessions.m, sid)
		sessions.mu.Unlock()
		removeCredFile(sid)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/landingpage", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// ---------------------------------------------------------------------------
// CSRF (per-session token, constant-time compare)
// ---------------------------------------------------------------------------

func ensureCSRF(sess *session) string {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	if sess.csrf == "" {
		sess.csrf = newToken(16)
	}
	return sess.csrf
}

func requireCSRF(w http.ResponseWriter, r *http.Request, sess *session) bool {
	sent := r.Header.Get("X-Landing-CSRF")
	if sent == "" {
		sent = r.PostFormValue("csrf")
	}
	sessions.mu.Lock()
	want := sess.csrf
	sessions.mu.Unlock()
	if sent == "" || want == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(want)) != 1 {
		jsonErr(w, "Invalid CSRF token, reload the page", http.StatusForbidden)
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// per-session credential store (S2: AES-256-GCM at rest, AAD binds the sid)
// ---------------------------------------------------------------------------

type creds struct {
	Username string `json:"username"`
	APIKey   string `json:"api_key"`
	ClientIP string `json:"client_ip"`
	Sandbox  bool   `json:"sandbox"`
}

func masterKey() []byte {
	if hexKey := os.Getenv("MAIL_MASTER_KEY"); hexKey != "" {
		if b, err := hex.DecodeString(hexKey); err == nil && len(b) == 32 {
			return b
		}
		log.Fatal("MAIL_MASTER_KEY set but not 64 hex chars (32 bytes)")
	}
	path := filepath.Join(credDir(), ".masterkey")
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		return b
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand unavailable: %v", err)
	}
	if err := os.MkdirAll(credDir(), 0o700); err != nil {
		log.Fatalf("credential dir unavailable: %v", err)
	}
	_ = os.Chmod(credDir(), 0o700)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		log.Fatalf("cannot persist master key: %v", err)
	}
	return b
}

var (
	gcmOnce sync.Once
	gcmAEAD cipher.AEAD
)

func aead() cipher.AEAD {
	gcmOnce.Do(func() {
		block, err := aes.NewCipher(masterKey())
		if err != nil {
			log.Fatalf("aes: %v", err)
		}
		g, err := cipher.NewGCM(block)
		if err != nil {
			log.Fatalf("gcm: %v", err)
		}
		gcmAEAD = g
	})
	return gcmAEAD
}

func credPath(sid string) string {
	h := sha256.Sum256([]byte(sid))
	return filepath.Join(credDir(), "namecheap_"+hex.EncodeToString(h[:8])+".json")
}

func removeCredFile(sid string) {
	_ = os.Remove(credPath(sid))
}

func storeCreds(sid string, c creds) error {
	pt, err := json.Marshal(c)
	if err != nil {
		return err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := aead().Seal(nonce, nonce, pt, []byte(sid)) // AAD = session id
	if err := os.MkdirAll(credDir(), 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(credDir(), "."+filepath.Base(credPath(sid))+"."+newToken(6))
	if err := os.WriteFile(tmp, ct, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, credPath(sid))
}

func readCreds(sid string) (*creds, error) {
	ct, err := os.ReadFile(credPath(sid))
	if err != nil {
		return nil, err
	}
	g := aead()
	if len(ct) < g.NonceSize() {
		return nil, errors.New("credential record too short")
	}
	nonce, body := ct[:g.NonceSize()], ct[g.NonceSize():]
	pt, err := g.Open(nil, nonce, body, []byte(sid)) // AAD must match
	if err != nil {
		return nil, err
	}
	var c creds
	if err := json.Unmarshal(pt, &c); err != nil || c.Username == "" || c.APIKey == "" {
		return nil, errors.New("invalid credential record")
	}
	return &c, nil
}

// reapOrphans hourly-deletes per-session credential files untouched for credTTL.
func reapOrphans(stop <-chan struct{}) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			entries, err := os.ReadDir(credDir())
			if err != nil {
				continue
			}
			cut := time.Now().Add(-credTTL)
			for _, e := range entries {
				name := e.Name()
				if !strings.HasPrefix(name, "namecheap_") || !strings.HasSuffix(name, ".json") {
					continue
				}
				info, err := e.Info()
				if err == nil && info.ModTime().Before(cut) {
					_ = os.Remove(filepath.Join(credDir(), name))
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// rate limiting (S3: per IP in addition to per session)
// ---------------------------------------------------------------------------

type ipLimiter struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

var ipHits = &ipLimiter{m: map[string][]time.Time{}}

func registerFailedAttempt(w http.ResponseWriter, sid string, sess *session, ip string) bool {
	now := time.Now()
	sessions.mu.Lock()
	sess.attempts = append(filterRecent(sess.attempts, now), now)
	overSession := len(sess.attempts) > maxAttempts
	sessions.mu.Unlock()

	ipHits.mu.Lock()
	ipHits.m[ip] = append(filterRecent(ipHits.m[ip], now), now)
	overIP := len(ipHits.m[ip]) > maxAttempts
	ipHits.mu.Unlock()

	if overSession || overIP {
		jsonErr(w, "Too many failed attempts, wait 10 minutes and retry", http.StatusTooManyRequests)
		return false
	}
	return true
}

func clearFailedAttempts(sess *session) {
	sessions.mu.Lock()
	sess.attempts = nil
	sessions.mu.Unlock()
}

func filterRecent(ts []time.Time, now time.Time) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if now.Sub(t) < attemptWindow {
			out = append(out, t)
		}
	}
	return out
}

// clientIP resolves the real client address for the IP limiter (S3).
func clientIP(r *http.Request) string {
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return strings.Trim(host, "[]")
}

// detectEgressIP finds the server's own public egress IP (the value Namecheap
// whitelists). Cached in CRED_DIR so /api/status never has to disclose it (S4).
func detectEgressIP() string {
	cache := filepath.Join(credDir(), ".publicip")
	if b, err := os.ReadFile(cache); err == nil {
		if ip := strings.TrimSpace(string(b)); validPublicIP(ip) {
			return ip
		}
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	ip := strings.TrimSpace(string(b))
	if !validPublicIP(ip) {
		return ""
	}
	_ = os.MkdirAll(credDir(), 0o700)
	_ = os.WriteFile(cache, []byte(ip+"\n"), 0o600)
	return ip
}

func validPublicIP(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && !parsed.IsLoopback() && !parsed.IsPrivate() && !parsed.IsUnspecified() && !parsed.IsLinkLocalUnicast()
}

// ---------------------------------------------------------------------------
// Namecheap client (unchanged behavior: XML form-encoded, stdlib encoding/xml)
// ---------------------------------------------------------------------------

// ncEnvelope parses the whole Namecheap response with encoding/xml — a real
// parser on untrusted input, not hand-rolled string indexing (CISO: robust
// against malformed/tag-bomb responses).
type ncEnvelope struct {
	Errors struct {
		Error []struct {
			Number  string `xml:"Number,attr"`
			Message string `xml:",chardata"`
		} `xml:"Error"`
	} `xml:"Errors"`
	CommandResponse struct {
		DomainList struct {
			Domain []struct {
				Name       string `xml:"Name,attr"`
				Created    string `xml:"Created,attr"`
				Expires    string `xml:"Expires,attr"`
				IsExpired  string `xml:"IsExpired,attr"`
				IsLocked   string `xml:"IsLocked,attr"`
				AutoRenew  string `xml:"AutoRenew,attr"`
				WhoisGuard string `xml:"WhoisGuard,attr"`
			} `xml:"Domain"`
		} `xml:"DomainGetListResult"`
		DNSHosts struct {
			Host []struct {
				HostID  string `xml:"HostID,attr"`
				Name    string `xml:"Name,attr"`
				Type    string `xml:"Type,attr"`
				Address string `xml:"Address,attr"`
				MXPref  string `xml:"MXPref,attr"`
				TTL     string `xml:"TTL,attr"`
			} `xml:"host"`
		} `xml:"DomainDNSGetHostsResult"`
	} `xml:"CommandResponse"`
}

type ncResult struct {
	OK    bool
	Env   *ncEnvelope
	Error string
	Code  string
	HTTP  int
}

func ncCall(c creds, command string, extra url.Values) ncResult {
	params := url.Values{
		"ApiUser":  {c.Username},
		"ApiKey":   {c.APIKey},
		"UserName": {c.Username},
		"Command":  {command},
		"ClientIp": {c.ClientIP},
	}
	for k, vs := range extra {
		for _, v := range vs {
			params.Add(k, v)
		}
	}
	client := &http.Client{Timeout: ncTotalTO}
	req, err := http.NewRequest(http.MethodPost, ncAPIURL(c.Sandbox), strings.NewReader(params.Encode()))
	if err != nil {
		return ncResult{Error: "Namecheap request build failed"}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return ncResult{Error: "Namecheap API unreachable, check server network"}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB cap on untrusted responses
	if err != nil {
		return ncResult{Error: "Namecheap API unreachable, check server network"}
	}
	env := &ncEnvelope{}
	if err := xml.Unmarshal(body, env); err != nil {
		return ncResult{Error: "Invalid Namecheap API response", HTTP: resp.StatusCode}
	}
	res := ncResult{HTTP: resp.StatusCode, Env: env}
	if len(env.Errors.Error) > 0 {
		e := env.Errors.Error[0]
		num, msg := e.Number, strings.TrimSpace(e.Message)
		if num == "1011031" || strings.Contains(strings.ToLower(msg), "not whitelisted") {
			ip := c.ClientIP
			if ip == "" {
				ip = "unknown"
			}
			msg = "IP not whitelisted in Namecheap: add " + ip + " under Profile > Tools > API Access"
		}
		res.Error, res.Code = msg, num
		return res
	}
	res.OK = true
	return res
}

func ncDomains(c creds) ([]map[string]any, *ncResult) {
	domains := []map[string]any{}
	for page := 1; page <= ncMaxPages; page++ {
		extra := url.Values{"Page": {fmt.Sprint(page)}, "PageSize": {fmt.Sprint(ncPageSize)}}
		res := ncCall(c, "namecheap.domains.getList", extra)
		if !res.OK {
			return nil, &res
		}
		nodes := res.Env.CommandResponse.DomainList.Domain
		for _, n := range nodes {
			domains = append(domains, map[string]any{
				"name":       n.Name,
				"created":    n.Created,
				"expires":    n.Expires,
				"expired":    n.IsExpired == "true",
				"locked":     n.IsLocked == "true",
				"autorenew":  n.AutoRenew == "true",
				"whoisguard": orDefault(n.WhoisGuard, "NOTFOUND"),
			})
		}
		if len(nodes) < ncPageSize {
			break
		}
	}
	return domains, nil
}

func ncDNSHosts(c creds, domain string) ([]map[string]any, *ncResult) {
	res := ncCall(c, "namecheap.domains.dns.getHosts", url.Values{"DomainName": {domain}})
	if !res.OK {
		return nil, &res
	}
	hosts := []map[string]any{}
	for _, n := range res.Env.CommandResponse.DNSHosts.Host {
		hosts = append(hosts, map[string]any{
			"id":      n.HostID,
			"name":    n.Name,
			"type":    n.Type,
			"address": n.Address,
			"mx_pref": orDefault(n.MXPref, "10"),
			"ttl":     orDefault(n.TTL, "1800"),
		})
	}
	return hosts, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ---------------------------------------------------------------------------
// mailcow proxy (MCP tools)
// ---------------------------------------------------------------------------

func mailcowCall(verb, path string, body any) (map[string]any, error) {
	base := strings.TrimRight(env("MAILCOW_API_URL", "http://nginx"), "/")
	key := os.Getenv("MAILCOW_API_KEY")
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(verb, base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: mcTotalTO}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mailcow API unreachable at %s", base)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mailcow API unreachable at %s", base)
	}
	var data map[string]any
	_ = json.Unmarshal(b, &data)
	return data, nil
}

// ---------------------------------------------------------------------------
// audit log (secret-free, structured — journald/docker logs is the operator's pipeline)
// ---------------------------------------------------------------------------

// auditLog writes a structured audit line to stdout. Credentials and API keys
// never land here — only event, outcome, tool/domain, client IP.
func auditLog(event string, fields map[string]any) {
	rec := make(map[string]any, len(fields)+2)
	rec["event"] = event
	rec["ts"] = time.Now().UTC().Format(time.RFC3339)
	for k, v := range fields {
		rec[k] = v
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	log.Printf("[audit] %s", b)
}

// ---------------------------------------------------------------------------
// handlers
// ---------------------------------------------------------------------------

func requireNCSession(w http.ResponseWriter, r *http.Request) (sid string, sess *session, c *creds, ok bool) {
	sid, sess = ensureSession(w, r)
	if !sess.connected {
		jsonErr(w, "Not connected, connect a Namecheap API key first", http.StatusUnauthorized)
		return "", nil, nil, false
	}
	cc, err := readCreds(sid)
	if err != nil {
		sessions.mu.Lock()
		sess.connected = false
		sess.username = ""
		sessions.mu.Unlock()
		jsonErr(w, "Stored connection is gone, connect again", http.StatusUnauthorized)
		return "", nil, nil, false
	}
	return sid, sess, cc, true
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	_, sess := ensureSession(w, r)
	sessions.mu.Lock()
	connected := sess.connected
	username := sess.username
	sessions.mu.Unlock()
	var c *creds
	if connected {
		sid, _ := getSession(r)
		var err error
		if sid != "" {
			c, err = readCreds(sid)
		}
		if err != nil || sid == "" {
			connected = false
			username = ""
		}
	}
	jsonOut(w, map[string]any{
		"ok":            true,
		"connected":     connected && c != nil,
		"username":      mapNil(username),
		"sandbox":       c != nil && c.Sandbox,
		"csrf":          ensureCSRF(sess),
		"mailcow_origin": mailcowOrigin(r),
	})
}

func mailcowOrigin(r *http.Request) string {
	if o := os.Getenv("MAILCOW_UI_ORIGIN"); o != "" {
		return strings.TrimRight(o, "/")
	}
	host := r.Host
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(strings.Trim(host, "[]"), ":") {
		host = host[:i]
	}
	if host == "" {
		return ""
	}
	return "https://" + host
}

func mapNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		jsonErr(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	sid, sess := ensureSession(w, r)
	if !requireCSRF(w, r, sess) {
		return
	}
	username := strings.TrimSpace(r.PostFormValue("username"))
	apiKey := r.PostFormValue("api_key")
	formIP := strings.TrimSpace(r.PostFormValue("client_ip"))
	sandbox := r.PostFormValue("use_sandbox") == "1"

	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`).MatchString(username) {
		jsonErr(w, "Invalid Namecheap username", http.StatusBadRequest)
		return
	}
	if !regexp.MustCompile(`^[A-Za-z0-9-]{8,128}$`).MatchString(apiKey) {
		jsonErr(w, "Invalid API key format", http.StatusBadRequest)
		return
	}
	ip := formIP
	if ip == "" {
		ip = detectEgressIP() // Namecheap whitelists the SERVER egress IP, not the browser's
	}

	if !registerFailedAttempt(w, sid, sess, ip) {
		return
	}

	c := creds{Username: username, APIKey: apiKey, ClientIP: ip, Sandbox: sandbox}
	res := ncCall(c, "namecheap.domains.getList", url.Values{"Page": {"1"}, "PageSize": {"5"}})
	if !res.OK {
		auditLog("namecheap_connect_failed", map[string]any{"ip": ip, "nc_code": res.Code})
		jsonErr(w, res.Error, http.StatusUnauthorized)
		return
	}
	clearFailedAttempts(sess)
	// Rotate FIRST, then store under the NEW sid: rotateSession invalidates the
	// old sid AND its credential file — storing before rotating destroyed the
	// just-written credentials on every successful connect (P0, review CTO 19/10).
	newSID := rotateSession(w, r, sess)
	if err := storeCreds(newSID, c); err != nil {
		jsonErr(w, "Credential storage unavailable", http.StatusInternalServerError)
		return
	}
	sessions.mu.Lock()
	sessions.m[newSID].connected = true
	sessions.m[newSID].username = username
	sessions.m[newSID].expires = time.Now().Add(sessionTTL)
	sessions.mu.Unlock()
	auditLog("namecheap_connect", map[string]any{"ok": true, "username": username, "sandbox": sandbox, "ip": ip})
	jsonOut(w, map[string]any{"ok": true, "username": username, "sandbox": sandbox})
}

func handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	_, sess := ensureSession(w, r)
	if !requireCSRF(w, r, sess) {
		return
	}
	dropSession(w, r)
	auditLog("namecheap_disconnect", map[string]any{"ok": true, "ip": clientIP(r)})
	jsonOut(w, map[string]any{"ok": true})
}

func handleDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	_, _, c, ok := requireNCSession(w, r)
	if !ok {
		return
	}
	domains, fail := ncDomains(*c)
	if fail != nil {
		jsonErr(w, fail.Error, http.StatusBadGateway)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "domains": domains})
}

// ---------------------------------------------------------------------------
// environment choice — shared vs dedicated, persisted per account (20/10)
// ---------------------------------------------------------------------------

// envPath keys the choice file to the account (plain JSON — a plan preference,
// not a credential). Same dir + hashing pattern as the per-session creds.
func envPath(username string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username))))
	return filepath.Join(env("LANDING_CRED_DIR", "."), fmt.Sprintf("env_%s.json", hex.EncodeToString(h[:16])))
}

// handleEnv serves GET/POST api/env: the shared/dedicated environment choice,
// stored per account so it survives restarts and sessions (billing decision).
func handleEnv(w http.ResponseWriter, r *http.Request) {
	sid, sess := getSession(r)
	if sid == "" || !sess.connected || sess.username == "" {
		jsonErr(w, "Not connected — sign in with your API key first", http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet {
		data, err := os.ReadFile(envPath(sess.username))
		if err != nil {
			jsonOut(w, map[string]any{"ok": true, "env": nil})
			return
		}
		var saved struct {
			Env string `json:"env"`
		}
		_ = json.Unmarshal(data, &saved)
		jsonOut(w, map[string]any{"ok": true, "env": saved.Env})
		return
	}
	if r.Method != http.MethodPost {
		jsonErr(w, "GET or POST required", http.StatusMethodNotAllowed)
		return
	}
	if !requireCSRF(w, r, sess) {
		return
	}
	var payload struct {
		Env string `json:"env"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || (payload.Env != "shared" && payload.Env != "dedicated") {
		jsonErr(w, "env must be 'shared' or 'dedicated'", http.StatusBadRequest)
		return
	}
	body := fmt.Sprintf(`{"env":%q,"ts":%q}`, payload.Env, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(envPath(sess.username), []byte(body), 0600); err != nil {
		jsonErr(w, "Could not save the environment choice", http.StatusInternalServerError)
		return
	}
	auditLog("env_choice", map[string]any{"ok": true, "env": payload.Env})
	jsonOut(w, map[string]any{"ok": true, "env": payload.Env})
}

func handleDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	_, _, c, ok := requireNCSession(w, r)
	if !ok {
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if !validDomain(domain) {
		jsonErr(w, "Invalid domain name", http.StatusBadRequest)
		return
	}
	hosts, fail := ncDNSHosts(*c, domain)
	if fail != nil {
		jsonErr(w, fail.Error, http.StatusBadGateway)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "domain": domain, "hosts": hosts})
}

// ---------------------------------------------------------------------------
// MCP endpoint (JSON-RPC 2.0, Bearer key — unchanged behavior)
// ---------------------------------------------------------------------------

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  map[string]any  `json:"params"`
	ID      json.RawMessage `json:"id"`
}

func mcpTools() []map[string]any {
	empty := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	return []map[string]any{
		{
			"name":        "list_domains",
			"description": "List all mail domains managed by this multi.ai.inbox (mailcow) instance, with quota, alias limits, active and relay state.",
			"inputSchema": empty,
		},
		{
			"name":        "get_domain_settings",
			"description": "Get the full settings of a single mail domain: quota, max quota, aliases, mailboxes, DKIM, relay state, active state.",
			"inputSchema": map[string]any{
				"type": "object", "required": []string{"domain"},
				"properties":             map[string]any{"domain": map[string]any{"type": "string", "description": "Domain name, e.g. example.com"}},
				"additionalProperties":   false,
			},
		},
		{
			"name":        "set_domain_settings",
			"description": "Update settings of a single mail domain. Only include the attributes to change, e.g. {\"active\":\"1\"} to activate, {\"defquota\":\"3072\"} for default quota in MiB, {\"backupmx\":\"1\"} to relay, {\"rl_value\":\"10\",\"rl_frame\":\"s\"} for rate limit.",
			"inputSchema": map[string]any{
				"type": "object", "required": []string{"domain", "attrs"},
				"properties": map[string]any{
					"domain": map[string]any{"type": "string", "description": "Domain name to update, e.g. example.com"},
					"attrs":  map[string]any{"type": "object", "description": "Attributes to change, as accepted by the mailcow /api/v1/edit/domain attr object"},
				},
				"additionalProperties": false,
			},
		},
	}
}

func mcpToolCall(name string, args map[string]any) (map[string]any, error) {
	domain := strings.ToLower(strings.TrimSpace(str(args["domain"])))
	if domain != "" && !validDomain(domain) {
		return nil, fmt.Errorf("Invalid domain name: %s", domain)
	}
	switch name {
	case "list_domains":
		return mailcowCall(http.MethodGet, "/api/v1/get/domain/all", nil)
	case "get_domain_settings":
		if domain == "" {
			return nil, errors.New("get_domain_settings requires a valid domain")
		}
		return mailcowCall(http.MethodGet, "/api/v1/get/domain/"+url.PathEscape(domain), nil)
	case "set_domain_settings":
		attrs, ok := args["attrs"].(map[string]any)
		if !ok || domain == "" {
			return nil, errors.New("set_domain_settings requires a valid domain and an attrs object")
		}
		return mailcowCall(http.MethodPost, "/api/v1/edit/domain", map[string]any{"items": []string{domain}, "attr": attrs})
	}
	return nil, errUnknownTool
}

var errUnknownTool = errors.New("unknown tool")

func str(v any) string {
	s, _ := v.(string)
	return s
}

func rpcResult(id json.RawMessage, result map[string]any) map[string]any {
	var idVal any
	if len(id) > 0 {
		_ = json.Unmarshal(id, &idVal)
	}
	return map[string]any{"jsonrpc": "2.0", "id": idVal, "result": result}
}

func rpcError(id json.RawMessage, code int, message string) map[string]any {
	var idVal any
	if len(id) > 0 {
		_ = json.Unmarshal(id, &idVal)
	}
	return map[string]any{"jsonrpc": "2.0", "id": idVal, "error": map[string]any{"code": code, "message": message}}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func handleMCP(w http.ResponseWriter, r *http.Request) {
	auditLog("mcp_request", map[string]any{"ip": clientIP(r)}) // includes unauthorized attempts
	expected := os.Getenv("MAILCOW_API_KEY")
	provided := ""
	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			provided = strings.TrimSpace(parts[1])
		}
	}
	if expected == "" || provided == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		jsonErr(w, `Unauthorized: send "Authorization: Bearer <key>" (read-write key from mailcow.conf)`, http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		jsonErr(w, "POST required, this endpoint speaks JSON-RPC 2.0 (MCP Streamable HTTP)", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		jsonErr(w, "Invalid JSON-RPC 2.0 request", http.StatusBadRequest)
		return
	}
	var req rpcReq
	if err := json.Unmarshal(body, &req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		jsonErr(w, "Invalid JSON-RPC 2.0 request", http.StatusBadRequest)
		return
	}
	switch req.Method {
	case "initialize":
		writeJSON(w, http.StatusOK, rpcResult(req.ID, map[string]any{
			"protocolVersion": mcpProtoVer,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "multi-ai-inbox-mcp", "title": "multi.ai.inbox MCP", "version": "1.0.0"},
			"instructions":    "Tools read and update mail domain settings of this multi.ai.inbox instance via the mailcow REST API.",
		}))
	case "tools/list":
		writeJSON(w, http.StatusOK, rpcResult(req.ID, map[string]any{"tools": mcpTools()}))
	case "tools/call":
		name := str(req.Params["name"])
		args, _ := req.Params["arguments"].(map[string]any)
		data, err := mcpToolCall(name, args)
		auditLog("mcp_tool_call", map[string]any{
			"tool":   name,
			"domain": strings.ToLower(strings.TrimSpace(str(args["domain"]))),
			"ok":     err == nil,
			"ip":     clientIP(r),
		})
		if errors.Is(err, errUnknownTool) {
			writeJSON(w, http.StatusOK, rpcResult(req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "Unknown tool: " + name}}, "isError": true,
			}))
			return
		}
		if err != nil {
			writeJSON(w, http.StatusOK, rpcResult(req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}}, "isError": true,
			}))
			return
		}
		pretty, _ := json.MarshalIndent(data, "", "  ")
		writeJSON(w, http.StatusOK, rpcResult(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(pretty)}},
		}))
	default:
		if strings.HasPrefix(req.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, http.StatusOK, rpcError(req.ID, -32601, "Method not found: "+req.Method))
	}
}

// ---------------------------------------------------------------------------
// gateway domain settings — per-domain read + write (live 20/10)
// ---------------------------------------------------------------------------

// gwDomainRE: FQDN-safe domain names only (no path tricks into panel calls).
var gwDomainRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// ncOwnedDomains is a test hook over ncDomains so ownership checks can be
// mocked without hitting the Namecheap API (same pattern as the panel mock).
var ncOwnedDomains = ncDomains

// ownedDomain reports whether the FQDN is on the tenant's account
// (case-insensitive match over the Namecheap domain list).
func ownedDomain(domain string, owned []map[string]any) bool {
	for _, d := range owned {
		if name, _ := d["name"].(string); strings.EqualFold(name, domain) {
			return true
		}
	}
	return false
}

// gwSessionDomain validates the path domain and the connected session; it
// writes the error response and returns ok=false when either fails.
// Cross-tenant scoping (20/10): the path domain must belong to the connected
// tenant's Namecheap account — fail-closed, a shared-instance tenant can't
// read or write another tenant's gateway settings.
func gwSessionDomain(w http.ResponseWriter, r *http.Request) (string, *session, bool) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if !gwDomainRE.MatchString(domain) {
		jsonErr(w, "Invalid domain name", http.StatusBadRequest)
		return "", nil, false
	}
	sid, sess := getSession(r)
	if sid == "" || !sess.connected {
		jsonErr(w, "Not connected — sign in with your API key first", http.StatusUnauthorized)
		return "", nil, false
	}
	c, err := readCreds(sid)
	if err != nil {
		jsonErr(w, "Session credentials missing — reconnect", http.StatusUnauthorized)
		return "", nil, false
	}
	owned, fail := ncOwnedDomains(*c)
	if fail != nil {
		jsonErr(w, "Cannot verify domain ownership right now — try again", http.StatusBadGateway)
		return "", nil, false
	}
	if !ownedDomain(domain, owned) {
		jsonErr(w, "Domain is not on your account", http.StatusForbidden)
		return "", nil, false
	}
	return domain, sess, true
}

// handleGatewayDomainGet serves GET api/gateway/domains/<domain>: the mail
// platform's record for the domain (fqdn, branding ← description, quotas MiB).
// 404 when the domain is not provisioned yet — the UI surfaces that honestly.
func handleGatewayDomainGet(w http.ResponseWriter, r *http.Request) {
	domain, _, ok := gwSessionDomain(w, r)
	if !ok {
		return
	}
	_, body, err := gwMailcowCall(http.MethodGet, "/api/v1/get/domain/"+domain, nil)
	if err != nil {
		jsonErr(w, fmt.Sprintf("mail platform did not answer: %v", err), http.StatusBadGateway)
		return
	}
	var d mcDomain
	if err := json.Unmarshal(body, &d); err != nil || d.DomainName == "" {
		jsonErr(w, "Domain is not provisioned on the mail platform yet", http.StatusNotFound)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "domain": map[string]any{
		"fqdn":       d.DomainName,
		"branding":   d.Description,
		"defquota":   d.DefQuota,
		"maxquota":   d.MaxQuota,
		"ptr_target": "",
	}})
}

// handleGatewayDomainPatch serves PATCH api/gateway/domains/<domain> (session
// + CSRF). Only fields the mail API accepts are written: maxquota/defquota
// (MiB) and branding → description. ptr_target (rDNS) is set at the network
// provider — sent values surface as a warning, never a fake success.
func handleGatewayDomainPatch(w http.ResponseWriter, r *http.Request) {
	domain, sess, ok := gwSessionDomain(w, r)
	if !ok {
		return
	}
	if !requireCSRF(w, r, sess) {
		return
	}
	// Plan caps (20/10): the shared tiers limit the number of domains the
	// tenant manages — enforced here, the write path, not just priced.
	plan := planFor(sess.username)
	known := provisionedDomains(sess.username)
	isNewDomain := true
	for _, d := range known {
		if strings.EqualFold(d, domain) {
			isNewDomain = false
			break
		}
	}
	if isNewDomain && plan.MaxDomains > 0 && len(known) >= plan.MaxDomains {
		jsonErr(w, fmt.Sprintf("Plan limit reached — %s includes %d domains. Upgrade to manage more.", plan.Label, plan.MaxDomains), http.StatusPaymentRequired)
		return
	}
	var payload struct {
		Branding  string   `json:"branding"`
		PTRTarget string   `json:"ptr_target"`
		DefQuota  *float64 `json:"defquota"`
		MaxQuota  *float64 `json:"maxquota"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonErr(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if (payload.DefQuota != nil && *payload.DefQuota < 0) || (payload.MaxQuota != nil && *payload.MaxQuota < 0) {
		jsonErr(w, "Quotas cannot be negative", http.StatusBadRequest)
		return
	}
	attr := map[string]any{}
	if payload.MaxQuota != nil {
		attr["maxquota"] = fmt.Sprint(*payload.MaxQuota)
	}
	if payload.DefQuota != nil {
		attr["defquota"] = fmt.Sprint(*payload.DefQuota)
	}
	if payload.Branding != "" {
		attr["description"] = payload.Branding
	}
	var warnings []string
	if payload.PTRTarget != "" {
		warnings = append(warnings, "PTR (rDNS) is a network-level setting at your hosting provider — it cannot be changed here.")
	}
	record := map[string]any{
		"fqdn":       domain,
		"branding":   payload.Branding,
		"defquota":   payload.DefQuota,
		"maxquota":   payload.MaxQuota,
		"ptr_target": payload.PTRTarget,
	}
	if len(attr) == 0 {
		// nothing writable sent — honest no-op with the warning if any
		jsonOut(w, map[string]any{"ok": true, "domain": record, "warnings": warnings})
		return
	}
	_, body, err := gwMailcowCall(http.MethodPost, "/api/v1/edit/domain", map[string]any{
		"items": []string{domain},
		"attr":  attr,
	})
	if err != nil {
		jsonErr(w, fmt.Sprintf("mail platform refused: %v", err), http.StatusBadGateway)
		return
	}
	if err := mcSuccess(body); err != nil {
		jsonErr(w, fmt.Sprintf("mail platform refused: %v", err), http.StatusBadGateway)
		return
	}
	if isNewDomain {
		addProvisionedDomain(sess.username, domain)
	}
	auditLog("domain_settings_patch", map[string]any{"ok": true, "domain": domain, "fields": attr})
	jsonOut(w, map[string]any{"ok": true, "domain": record, "warnings": warnings})
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	stop := make(chan struct{})
	defer close(stop)
	go reapOrphans(stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/connect", handleConnect)
	mux.HandleFunc("/api/disconnect", handleDisconnect)
	mux.HandleFunc("/api/domains", handleDomains)
	mux.HandleFunc("/api/env", handleEnv)
	mux.HandleFunc("/api/dns", handleDNS)
	mux.HandleFunc("/api/mcp", handleMCP)
	registerGatewayRoutes(mux, MailcowRegistrar{}) // gateway agenti con chiavi scoped (decisione founder 19/10)
	mux.HandleFunc("GET /api/gateway/domains/{domain}", handleGatewayDomainGet)
	mux.HandleFunc("PATCH /api/gateway/domains/{domain}", handleGatewayDomainPatch)
	mux.HandleFunc("POST /api/checkout", handleCheckout) // self-serve subscriptions (Mollie, 21/10)
	mux.HandleFunc("POST /api/checkout/webhook", handleCheckoutWebhook)
	mux.HandleFunc("GET /api/checkout/state", handleCheckoutState)

	srv := &http.Server{
		Addr:              internalAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("multi.ai.inbox landing API (%s) listening on %s", protocolOrigin, internalAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("listen: %v", err)
	}
}
