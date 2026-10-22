package main

// Founder console data: mail + infrastructure metrics for the control
// dashboard. Both endpoints are admin-gated (isAdmin, env LANDING_ADMIN_USER,
// fail-closed) — anonymous and non-admin sessions get the honest 403 and
// fetch no scoped data.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// requireAdminSession mirrors handleStatus's connection resolution: the
// session must be connected, still resolve to stored creds, and the connected
// username must match LANDING_ADMIN_USER. Anything else is the honest 403.
func requireAdminSession(w http.ResponseWriter, r *http.Request) (string, *session, bool) {
	sid, sess := ensureSession(w, r)
	sessions.mu.Lock()
	connected, username := sess.connected, sess.username
	sessions.mu.Unlock()
	alive := false
	if connected {
		if _, err := readCreds(sid); err == nil {
			alive = true
		}
	}
	if !isAdmin(alive, username) {
		jsonErr(w, "Admin session required", http.StatusForbidden)
		return "", nil, false
	}
	return sid, sess, true
}

// handleAdminMetrics serves GET /api/admin/metrics (dashboard stat strip).
func handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if _, _, ok := requireAdminSession(w, r); !ok {
		return
	}

	mail := map[string]any{"domains": 0, "mailboxes": 0, "mailboxes_by_domain": []map[string]any{}}
	if err := fillMailMetrics(mail); err != nil {
		// Honest: the mail platform is the data source — surface the gap.
		mail["available"] = false
		mail["reason"] = err.Error()
	} else {
		mail["available"] = true
	}

	sessions.mu.Lock()
	active := len(sessions.m)
	sessions.mu.Unlock()

	states := provisionedDomainStates()
	enabled := provisioningEnabled()
	provisioning := map[string]any{
		"enabled": enabled,
		"pending": countPending(states),
		"states":  states,
	}

	auditRing.Lock()
	recent := make([]map[string]any, 0, 20)
	for i := len(auditRing.evs) - 1; i >= 0 && len(recent) < 20; i-- {
		ev := auditRing.evs[i]
		evn, _ := ev["event"].(string)
		title, _ := notificationText(evn, ev)
		recent = append(recent, map[string]any{
			"ts":     ev["ts"],
			"event":  evn,
			"detail": title,
		})
	}
	auditRing.Unlock()

	jsonOut(w, map[string]any{
		"ok":           true,
		"admin":        true,
		"mail":         mail,
		"sessions":     map[string]any{"active": active},
		"provisioning": provisioning,
		"audit":        map[string]any{"recent": recent},
	})
}

// fillMailMetrics counts domains + mailboxes from the mail platform API. The
// mailbox listing answers with an ARRAY of rows when any exist and with an
// empty OBJECT ({} — not []) when none do: accept both, anything else is
// unreadable.
func fillMailMetrics(mail map[string]any) error {
	status, body, err := gwMailcowCall(http.MethodGet, "/api/v1/get/domain/all", nil)
	if err != nil {
		return fmt.Errorf("mail platform did not answer: %v", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("mail platform answered %d", status)
	}
	var domains []mcDomain
	if err := json.Unmarshal(body, &domains); err != nil {
		var obj map[string]any
		if err2 := json.Unmarshal(body, &obj); err2 != nil || len(obj) != 0 {
			return fmt.Errorf("mail platform sent an unreadable domain listing")
		}
	}

	_, body, err = gwMailcowCall(http.MethodGet, "/api/v1/get/mailbox/all", nil)
	if err != nil {
		return fmt.Errorf("mail platform did not answer: %v", err)
	}
	var boxes []mcMailbox
	if err := json.Unmarshal(body, &boxes); err != nil {
		var obj map[string]any
		if err2 := json.Unmarshal(body, &obj); err2 != nil || len(obj) != 0 {
			return fmt.Errorf("mail platform sent an unreadable mailbox listing")
		}
	}

	byDomain := map[string]int{}
	for _, b := range boxes {
		address := b.Username
		if i := strings.Index(address, "@"); i >= 0 && i < len(address)-1 {
			byDomain[address[i+1:]]++
		}
	}
	out := make([]map[string]any, 0, len(byDomain))
	for _, d := range domains {
		n := byDomain[d.DomainName]
		out = append(out, map[string]any{"domain": d.DomainName, "mailboxes": n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["domain"].(string) < out[j]["domain"].(string) })

	mail["domains"] = len(domains)
	mail["mailboxes"] = len(boxes)
	mail["mailboxes_by_domain"] = out
	return nil
}

func countPending(states []map[string]any) int {
	n := 0
	for _, s := range states {
		if st, _ := s["state"].(string); st != "" && st != "done" && st != "active" && st != "running" {
			n++
		}
	}
	return n
}

// provisionedDomainStates lists every provisioned domain's state (admin view).
func provisionedDomainStates() []map[string]any {
	out := []map[string]any{}
	sessions.mu.Lock()
	users := make([]string, 0, len(sessions.m))
	for _, s := range sessions.m {
		if s.connected && s.username != "" {
			users = append(users, s.username)
		}
	}
	sessions.mu.Unlock()
	seen := map[string]bool{}
	for _, u := range users {
		for _, d := range provisionedDomains(u) {
			if seen[d] {
				continue
			}
			seen[d] = true
			state := "claimed"
			if job, err := readProvJob(u); err == nil && job != nil {
				state = job.State
			}
			out = append(out, map[string]any{"domain": d, "state": state})
		}
	}
	return out
}

// handleAdminInfra serves GET /api/admin/infra — what the app container can
// honestly measure: uptime, memory, disk on the credentials volume. Services
// health is not readable from here (no docker socket): the honest answer is
// available=false with the reason, not a fabricated list.
func handleAdminInfra(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if _, _, ok := requireAdminSession(w, r); !ok {
		return
	}

	upS := uptimeSeconds()
	memUsed, memTotal, memErr := meminfo()
	diskUsed, diskTotal, diskErr := diskUsage(credDir())

	mem := map[string]any{"available": memErr == nil}
	if memErr == nil {
		mem["used_mb"] = memUsed / 1024
		mem["total_mb"] = memTotal / 1024
		if memTotal > 0 {
			mem["pct"] = int(100 * memUsed / memTotal)
		}
	} else {
		mem["reason"] = memErr.Error()
	}
	disk := map[string]any{"available": diskErr == nil}
	if diskErr == nil {
		disk["used_gb"] = roundGB(diskUsed)
		disk["total_gb"] = roundGB(diskTotal)
		if diskTotal > 0 {
			disk["pct"] = int(100 * diskUsed / diskTotal)
		}
	} else {
		disk["reason"] = diskErr.Error()
	}

	jsonOut(w, map[string]any{
		"ok":       true,
		"uptime_s": upS,
		"disk":     disk,
		"memory":   mem,
		"services": map[string]any{
			"available": false,
			"reason":    "container health is not readable from the app container — check the platform logs or the provider console",
		},
	})
}

func uptimeSeconds() int64 {
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) > 0 {
			if f, err := strconv.ParseFloat(fields[0], 64); err == nil {
				return int64(f)
			}
		}
	}
	return 0
}

func meminfo() (usedKb, totalKb int64, err error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, fmt.Errorf("memory metrics unavailable (%v)", err)
	}
	var memAvailable, memTotal int64
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = meminfoKb(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailable = meminfoKb(line)
		}
	}
	if memTotal <= 0 {
		return 0, 0, fmt.Errorf("memory metrics unreadable")
	}
	return memTotal - memAvailable, memTotal, nil
}

func meminfoKb(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func diskUsage(dir string) (used, total uint64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(dir, &st); err != nil {
		return 0, 0, fmt.Errorf("disk metrics unavailable (%v)", err)
	}
	total = st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	used = total - free
	return used, total, nil
}

func roundGB(bytes uint64) float64 {
	return float64(bytes) / (1024 * 1024 * 1024)
}

// honest shape for the admin feed: title is the item text.
