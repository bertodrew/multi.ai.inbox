package main

// provisioning.go — FASE 1 provisioning pipeline (iaas-design.md §4). When a
// Mollie webhook arrives paid for a Dedicated/Dedicated XL plan, a
// provisioning job is queued as a 0600 JSON file in LANDING_CRED_DIR. The
// instance is then created against the Hetzner Cloud API by the in-process
// scan pass (runProvScan): queued → creating → active | failed. The pass is
// armed only when the provider token exists (fail-closed: no token, no
// provider call — the job stays honestly queued awaiting a worker). The
// mailcow install + DNS write + onboarding remain external (worker /
// founder — implementation status in iaas-design.md §4).
//
// Fail-closed: without PROVISIONING_ENABLED=1 nothing is queued, the
// persist-only flow stays untouched, and the webhook response reports
// provisioning="not_enabled". Shared plans never trigger (not_applicable).
//
// Vendor screen (iaas-design.md §6): Hetzner (DE) is the implemented API
// path; Exoscale (CH) is failed honestly as not implemented (501) —
// no US hyperscalers. Credentials (HCLOUD_TOKEN) travel only in the
// Authorization header, never in the job file and never in the client.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// provServerTypes maps a dedicated plan to its IaaS server type (Hetzner
// SKU default; the worker/deployment can override via tfvars). Resize path
// per iaas-design.md §2: dedicated ccx41→ccx42→ccx43 (maintenance window);
// dedicated-xl is the root-server tier.
var provServerTypes = map[string]string{
	"dedicated":    "ccx41",
	"dedicated-xl": "ccx63",
}

// provJobPath keys the provisioning job file to the account (plain JSON —
// the plan and job state, not a credential). Same dir + hashing pattern as
// checkoutPath; reapOrphans never touches prov_*.json (it only reaps
// namecheap_* files), so a job survives restarts until the worker settles it.
func provJobPath(username string) string {
	h := sha256.Sum256([]byte(username))
	return filepath.Join(env("LANDING_CRED_DIR", "."), fmt.Sprintf("prov_%s.json", hex.EncodeToString(h[:16])))
}

// provJob is one provisioning job: what the in-process scan (or an external
// worker) consumes (plan → server type, provider/location, mail FQDN) plus
// its lifecycle state. No secrets: API keys and SSH keys stay in the
// environment.
type provJob struct {
	Username   string `json:"username"`
	Plan       string `json:"plan"`
	Label      string `json:"label"`
	PaymentID  string `json:"payment_id"`
	ServerType string `json:"server_type"`
	Provider   string `json:"provider"`
	Location   string `json:"location"`
	Hostname   string `json:"hostname"`
	State      string `json:"state"` // queued | creating | active | failed (legacy file-worker: running | done)
	Attempts   int    `json:"attempts"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	ServerID   string `json:"server_id,omitempty"` // provider server id (hetzner)
	IP         string `json:"ip,omitempty"`        // provider public IPv4 on active
	Error      string `json:"error,omitempty"`     // provider/config failure reason
}

// provisioningEnabled is the fail-closed gate: only the exact value "1"
// arms the trigger. An unset, empty or partial value keeps the current
// persist-only flow.
func provisioningEnabled() bool {
	return os.Getenv("PROVISIONING_ENABLED") == "1"
}

// provMailHostname derives the instance FQDN from the tenant username
// (labels are lowercased; _ is not legal in a hostname → -). Empty unless
// PROVISIONING_BASE_DOMAIN is set — the worker then fails the job instead
// of inventing a domain.
func provMailHostname(username string) string {
	base := os.Getenv("PROVISIONING_BASE_DOMAIN")
	if base == "" {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(username, "_", "-")) + "." + strings.ToLower(base)
}

// provLocation resolves the target zone: Hetzner locations (nbg1/fsn1/hel1)
// by default, Exoscale CH zones (ch-gva-2/ch-dk-2) when the provider is
// exoscale — the explicit data-residency choice (iaas-design.md §6).
func provLocation() string {
	if l := os.Getenv("PROVISIONING_LOCATION"); l != "" {
		return l
	}
	if env("PROVISIONING_PROVIDER", "hetzner") == "exoscale" {
		return "ch-gva-2"
	}
	return "nbg1"
}

// readProvJob loads the account's provisioning job; (nil, nil) when none.
func readProvJob(username string) (*provJob, error) {
	data, err := os.ReadFile(provJobPath(username))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var j provJob
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// saveProvJob writes the job atomically (tmp + rename), 0600 — the worker
// never reads a half-written file, and the perms match the other
// per-account files in the cred dir.
func saveProvJob(j provJob) error {
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	path := provJobPath(j.Username)
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+newToken(6))
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// queueProvisioning queues (or re-queues) a dedicated instance job.
// Idempotent: Mollie retries the webhook, and every recurring monthly
// payment arrives paid — an existing queued/running/done job is left
// untouched (terraform is not re-run every month); only a failed job is
// re-queued. Returns the resulting state.
func queueProvisioning(username, planKey, paymentID string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("provisioning requires an account")
	}
	plan, ok := checkoutPlans[planKey]
	if !ok {
		return "", fmt.Errorf("provisioning not available for plan %q", planKey)
	}
	serverType, ok := provServerTypes[planKey]
	if !ok {
		return "", fmt.Errorf("no server type mapped for plan %q", planKey)
	}
	// Account spend guard (21/10): the budget holds even on a direct queue —
	// the webhook path checks it first, this is the second wall.
	if err := spendGuard(serverType); err != nil {
		return "", err
	}
	existing, err := readProvJob(username)
	if err != nil {
		return "", fmt.Errorf("provisioning job unreadable: %v", err)
	}
	if existing != nil && existing.State != "failed" {
		return existing.State, nil
	}
	job := provJob{
		Username:   username,
		Plan:       planKey,
		Label:      plan.Label,
		PaymentID:  paymentID,
		ServerType: serverType,
		Provider:   env("PROVISIONING_PROVIDER", "hetzner"),
		Location:   provLocation(),
		Hostname:   provMailHostname(username),
		State:      "queued",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if existing != nil { // re-queue of a failed job: keep history, drop stale output
		job.CreatedAt = existing.CreatedAt
		job.Attempts = existing.Attempts + 1
	}
	if err := saveProvJob(job); err != nil {
		return "", fmt.Errorf("provisioning job write failed: %v", err)
	}
	return job.State, nil
}

// triggerProvisioning queues the job in the background: the webhook answers
// immediately, the job file write + audit log land in the goroutine. With
// the provider token present the same goroutine runs one scan pass — the
// instance is created against the Hetzner API (queued → creating → active).
// Without a token nothing runs (fail-closed): the job stays honestly queued
// awaiting a worker — no fake success, no fake failure.
func triggerProvisioning(username, planKey, paymentID string) {
	go func() {
		state, err := queueProvisioning(username, planKey, paymentID)
		if err != nil {
			auditLog("provisioning_queue_failed", map[string]any{"username": username, "plan": planKey, "error": err.Error()})
			return
		}
		auditLog("provisioning_queued", map[string]any{"username": username, "plan": planKey, "state": state})
		if provToken() != "" {
			runProvScan()
		}
	}()
}

// handleProvisioningState serves GET api/provisioning/state (auth): the
// provisioning job of the connected account, for the UI/settings page.
// "none" when no job exists — the UI surfaces that honestly.
func handleProvisioningState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	sid, sess := getSession(r)
	if sid == "" || !sess.connected || sess.username == "" {
		jsonErr(w, "Not connected — sign in with your API key first", http.StatusUnauthorized)
		return
	}
	job, err := readProvJob(sess.username)
	if err != nil {
		jsonErr(w, "Provisioning state unreadable", http.StatusInternalServerError)
		return
	}
	if job == nil {
		jsonOut(w, map[string]any{"ok": true, "provisioning": "none", "provisioning_enabled": provisioningEnabled()})
		return
	}
	if job.State == "failed" {
		// Honest fail-closed: a failed job is a provisioning path that
		// cannot proceed — 501 when the provider isn't implemented, 503
		// for everything else (missing token, provider error, budget
		// wall). The body still carries the job for the UI.
		code := http.StatusServiceUnavailable
		if strings.Contains(job.Error, "non implementato") {
			code = http.StatusNotImplemented
		}
		provJSONStatus(w, code, map[string]any{
			"ok":                   false,
			"provisioning":         "failed",
			"error":                job.Error,
			"provisioning_enabled": provisioningEnabled(),
			"job":                  provJobJSON(job),
		})
		return
	}
	jsonOut(w, map[string]any{
		"ok":                   true,
		"provisioning":         job.State,
		"provisioning_enabled": provisioningEnabled(),
		"job":                  provJobJSON(job),
	})
}

// ---------------------------------------------------------------------------
// FASE 1 — provider API path (22/10, iaas-design.md §4): the in-process scan
// pass advances every pending job against the Hetzner Cloud API:
//
//	queued → creating (POST /servers accepted) → active (provider status
//	running) | failed (token missing, budget wall, provider error)
//
// Fail-closed, honest: without HCLOUD_TOKEN no provider call is made and the
// job is failed with an explicit message (the state endpoint surfaces 503);
// a provider this build does not implement is failed with 501 semantics. The
// token travels only in the Authorization header — never in the job file,
// never in a response body.
// ---------------------------------------------------------------------------

// provAPIBase: Hetzner Cloud API base URL (HCLOUD_API_URL overrides — the
// tests and a self-hosted proxy point here; production default is the
// public API).
func provAPIBase() string {
	if b := os.Getenv("HCLOUD_API_URL"); b != "" {
		return strings.TrimRight(b, "/")
	}
	return "https://api.hetzner.cloud/v1"
}

// provToken: the provider credential from the environment — HCLOUD_TOKEN for
// hetzner (the implemented path, default), EXOSCALE_API_KEY for exoscale.
// Empty means "not configured": the scan refuses to create (fail-closed).
func provToken() string {
	if env("PROVISIONING_PROVIDER", "hetzner") == "exoscale" {
		return os.Getenv("EXOSCALE_API_KEY")
	}
	return os.Getenv("HCLOUD_TOKEN")
}

// provPollTimeout / provPollInterval: the bounded create→active poll. A job
// still building past the deadline stays honestly "creating" (the next scan
// pass retries) — a poll timeout is never reported as success or failure.
func provPollTimeout() time.Duration {
	if v := os.Getenv("PROVISIONING_POLL_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 120 * time.Second
}

func provPollInterval() time.Duration {
	if v := os.Getenv("PROVISIONING_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 2 * time.Second
}

// provScanMu serializes scan passes: two concurrent scans must never create
// the same queued job twice (a double creation is a double-billing class of
// bug). Every job is re-read under the lock before it is acted on.
var provScanMu sync.Mutex

// provJSONStatus writes a JSON response with an explicit HTTP status —
// jsonOut always writes 200, and a failed provisioning path must surface
// 501/503 honestly.
func provJSONStatus(w http.ResponseWriter, status int, data map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(data)
}

// provJobJSON renders the job for the state endpoint (shared by the 200 and
// the 501/503 shape).
func provJobJSON(job *provJob) map[string]any {
	return map[string]any{
		"plan":        job.Plan,
		"label":       job.Label,
		"state":       job.State,
		"server_type": job.ServerType,
		"provider":    job.Provider,
		"location":    job.Location,
		"hostname":    job.Hostname,
		"server_id":   job.ServerID,
		"ip":          job.IP,
		"error":       job.Error,
		"attempts":    job.Attempts,
		"created_at":  job.CreatedAt,
		"updated_at":  job.UpdatedAt,
	}
}

// provServerResp is the Hetzner create/status response shape (server.id,
// server.status, server.public_net.ipv4.ip — IPv4 can be null).
type provServerResp struct {
	Server struct {
		ID        int64  `json:"id"`
		Status    string `json:"status"`
		PublicNet struct {
			IPv4 struct {
				IP string `json:"ip"`
			} `json:"ipv4"`
		} `json:"public_net"`
	} `json:"server"`
}

// provAPIRequest does one authenticated call against the provider API. The
// token rides only the Authorization header; non-2xx becomes an honest
// error carrying the provider status + trimmed body.
func provAPIRequest(method, path, token string, payload any) (*provServerResp, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, provAPIBase()+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hetzner API irraggiungibile: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("hetzner API %d: risposta illeggibile", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		if msg == "" {
			msg = "nessun dettaglio"
		}
		return nil, fmt.Errorf("hetzner API %d: %s", resp.StatusCode, msg)
	}
	var out provServerResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("hetzner API %d: risposta non interpretabile: %v", resp.StatusCode, err)
	}
	return &out, nil
}

// provCreatePayload builds the POST /servers body: the mail FQDN as name,
// the whitelisted server type, the resolved location, the install image and
// an account label. SSH keys come from PROVISIONING_SSH_KEY_IDS (Hetzner
// key ids, comma-separated) — credentials never travel in the job file.
func provCreatePayload(j *provJob) map[string]any {
	name := j.Hostname
	if name == "" {
		name = "mail-" + strings.ToLower(strings.ReplaceAll(j.Username, "_", "-"))
	}
	payload := map[string]any{
		"name":        name,
		"server_type": j.ServerType,
		"location":    j.Location,
		"image":       env("PROVISIONING_IMAGE", "ubuntu-24.04"),
		"labels":      map[string]string{"managed-by": "multiai-landing", "account": j.Username},
		"public_net":  map[string]any{"enable_ipv4": true, "enable_ipv6": true},
	}
	if ids := os.Getenv("PROVISIONING_SSH_KEY_IDS"); ids != "" {
		var keys []int
		for _, p := range strings.Split(ids, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				keys = append(keys, n)
			}
		}
		if len(keys) > 0 {
			payload["ssh_keys"] = keys
		}
	}
	return payload
}

// provScanCreate advances one queued job: honest walls first (provider not
// implemented → 501 semantics, missing token → 503, budget whitelist →
// spendGuard error), then the create call, then the bounded poll to active.
func provScanCreate(j *provJob) {
	if env("PROVISIONING_PROVIDER", "hetzner") != "hetzner" {
		j.State = "failed"
		j.Error = fmt.Sprintf("provisioning provider %q non implementato (501) — solo hetzner", j.Provider)
		_ = saveProvJob(*j)
		auditLog("provisioning_failed", map[string]any{"username": j.Username, "error": j.Error})
		return
	}
	token := provToken()
	if token == "" {
		j.State = "failed"
		j.Error = "HCLOUD_TOKEN non configurato — provisioning non disponibile (503); il job sarà ripreso dal prossimo pagamento"
		_ = saveProvJob(*j)
		auditLog("provisioning_failed", map[string]any{"username": j.Username, "error": j.Error})
		return
	}
	// Second wall (spend.go): whitelist + monthly budget — never bypassed.
	if err := spendGuard(j.ServerType); err != nil {
		j.State = "failed"
		j.Error = err.Error()
		_ = saveProvJob(*j)
		auditLog("provisioning_budget_blocked", map[string]any{"username": j.Username, "error": err.Error()})
		return
	}
	out, err := provAPIRequest(http.MethodPost, "/servers", token, provCreatePayload(j))
	if err != nil {
		j.State = "failed"
		j.Error = err.Error()
		_ = saveProvJob(*j)
		auditLog("provisioning_create_failed", map[string]any{"username": j.Username, "error": err.Error()})
		return
	}
	j.ServerID = strconv.FormatInt(out.Server.ID, 10)
	if ip := out.Server.PublicNet.IPv4.IP; ip != "" {
		j.IP = ip
	}
	if out.Server.Status == "running" {
		j.State = "active"
		_ = saveProvJob(*j)
		auditLog("provisioning_active", map[string]any{"username": j.Username, "server_id": j.ServerID})
		return
	}
	j.State = "creating"
	_ = saveProvJob(*j)
	auditLog("provisioning_creating", map[string]any{"username": j.Username, "server_id": j.ServerID, "status": out.Server.Status})
	provScanPoll(j, token)
}

// provScanPoll follows one creating job until the provider reports running
// (active), an error status (failed) or the deadline (stays creating — the
// next pass retries; a poll timeout is never success or failure).
func provScanPoll(j *provJob, token string) {
	deadline := time.Now().Add(provPollTimeout())
	for {
		out, err := provAPIRequest(http.MethodGet, "/servers/"+j.ServerID, token, nil)
		if err == nil {
			if ip := out.Server.PublicNet.IPv4.IP; ip != "" {
				j.IP = ip
			}
			switch out.Server.Status {
			case "running":
				j.State = "active"
				_ = saveProvJob(*j)
				auditLog("provisioning_active", map[string]any{"username": j.Username, "server_id": j.ServerID})
				return
			case "error":
				j.State = "failed"
				j.Error = fmt.Sprintf("hetzner server %s in errore (status error)", j.ServerID)
				_ = saveProvJob(*j)
				auditLog("provisioning_failed", map[string]any{"username": j.Username, "server_id": j.ServerID, "error": j.Error})
				return
			}
		}
		if time.Now().After(deadline) {
			note := "ancora in creazione al timeout di poll"
			if err != nil {
				note = "status illeggibile: " + err.Error()
			}
			auditLog("provisioning_poll_timeout", map[string]any{"username": j.Username, "server_id": j.ServerID, "note": note})
			return // still honestly "creating"
		}
		time.Sleep(provPollInterval())
	}
}

// provPendingJobs lists job files the scan can advance (queued | creating).
// A file that doesn't parse as a full provJob — the spend.go provScanJob
// shape (state + server_type) is the tolerant minimum — is audited and
// skipped, never advanced blind.
func provPendingJobs() []provJob {
	dir := env("LANDING_CRED_DIR", ".")
	files, err := filepath.Glob(filepath.Join(dir, "prov_*.json"))
	if err != nil {
		return nil
	}
	var jobs []provJob
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var j provJob
		if err := json.Unmarshal(data, &j); err != nil {
			var scan provScanJob
			_ = json.Unmarshal(data, &scan)
			auditLog("provisioning_scan_skipped", map[string]any{"file": filepath.Base(f), "state": scan.State})
			continue
		}
		if j.Username == "" {
			continue
		}
		if j.State == "queued" || j.State == "creating" {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

// runProvScan is one worker pass: it advances every pending job against the
// provider API and leaves terminal jobs (active/failed and the legacy
// file-worker running/done) untouched. Serialized by provScanMu.
func runProvScan() {
	provScanMu.Lock()
	defer provScanMu.Unlock()
	for _, pending := range provPendingJobs() {
		current, err := readProvJob(pending.Username)
		if err != nil || current == nil {
			continue // vanished or unreadable mid-scan: next pass
		}
		switch current.State {
		case "queued":
			provScanCreate(current)
		case "creating":
			if tok := provToken(); tok != "" && current.ServerID != "" {
				provScanPoll(current, tok)
			}
		}
	}
}
