package main

// provisioning.go — FASE 1 provisioning pipeline (iaas-design.md §4). When a
// Mollie webhook arrives paid for a Dedicated/Dedicated XL plan, a
// provisioning job is queued in the background. The real terraform apply
// NEVER runs in the web process: the job is a 0600 JSON file in
// LANDING_CRED_DIR consumed by an external worker (cron/systemd —
// landing/infra/worker.sh, documented in landing/infra/README.md).
//
// Fail-closed: without PROVISIONING_ENABLED=1 nothing is queued, the
// persist-only flow stays untouched, and the webhook response reports
// provisioning="not_enabled". Shared plans never trigger (not_applicable).
//
// Vendor screen (iaas-design.md §6): Hetzner (DE) or Exoscale (CH) only —
// no US hyperscalers. Credentials (hcloud token, SSH key) live in the
// worker environment, never in the job file and never in the client.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// provJob is one queued provisioning job: what the external worker consumes
// (plan → server type, provider/location, mail FQDN) plus its lifecycle
// state. No secrets: API keys and SSH keys stay in the worker env.
type provJob struct {
	Username   string `json:"username"`
	Plan       string `json:"plan"`
	Label      string `json:"label"`
	PaymentID  string `json:"payment_id"`
	ServerType string `json:"server_type"`
	Provider   string `json:"provider"`
	Location   string `json:"location"`
	Hostname   string `json:"hostname"`
	State      string `json:"state"` // queued | running | done | failed
	Attempts   int    `json:"attempts"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	IP         string `json:"ip,omitempty"`   // filled by the worker on done
	Error      string `json:"error,omitempty"` // filled by the worker on failed
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
// immediately, the job file write + audit log land in the goroutine. The
// real terraform apply runs only in the external worker — never here.
func triggerProvisioning(username, planKey, paymentID string) {
	go func() {
		state, err := queueProvisioning(username, planKey, paymentID)
		if err != nil {
			auditLog("provisioning_queue_failed", map[string]any{"username": username, "plan": planKey, "error": err.Error()})
			return
		}
		auditLog("provisioning_queued", map[string]any{"username": username, "plan": planKey, "state": state})
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
	jsonOut(w, map[string]any{
		"ok":                   true,
		"provisioning":         job.State,
		"provisioning_enabled": provisioningEnabled(),
		"job": map[string]any{
			"plan":        job.Plan,
			"label":       job.Label,
			"state":       job.State,
			"server_type": job.ServerType,
			"provider":    job.Provider,
			"location":    job.Location,
			"hostname":    job.Hostname,
			"ip":          job.IP,
			"error":       job.Error,
			"attempts":    job.Attempts,
			"created_at":  job.CreatedAt,
			"updated_at":  job.UpdatedAt,
		},
	})
}
