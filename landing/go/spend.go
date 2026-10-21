package main

// Spend guard (21/10, founder): the Hetzner account gets a monthly spend
// ceiling — malicious or runaway provisioning must not create damage.
// Hetzner has no native hard $-spend cap, so exposure is bounded here
// (fail-closed) AND by the project resource limits in the Hetzner Console
// (see landing/docs/iaas-design.md §7).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// instanceCostEUR: committed monthly cost per server type — Hetzner prices
// after the 2026-06-15 standardization (EUR, excl. IPv4; approximate, the
// September console lists CX23 at 5.99 "not available" and CPX12 as the
// cheapest orderable type at 11.99). Override at runtime with
// SPEND_INSTANCE_COSTS (JSON map) — prices move, the gate must not.
// instanceCosts builds a FRESH price map on every call — never a mutable
// global: the e2e ops override (SPEND_INSTANCE_COSTS) must not leak across
// requests, and tests must not contaminate each other (same lesson as the
// process-global ipHits limiter).
func instanceCosts() map[string]float64 {
	costs := map[string]float64{
		"cx23": 5.49, "cx33": 8.49, "cx43": 15.99, "cx53": 29.49,
		"cax11": 5.99, "cax21": 6.49, "cax31": 12.49, "cax41": 24.49,
		"cpx12": 11.99, "cpx22": 19.49, "cpx32": 10.99, "cpx42": 19.99,
		// CCX (dedicated vCPU, 42.99+) deliberately ABSENT: under the
		// account budget no dedicated-tier instance may be provisioned —
		// raise MAX_MONTHLY_SPEND_EUR and add the type via
		// SPEND_INSTANCE_COSTS when the first Dedicated customer pays.
	}
	if raw := os.Getenv("SPEND_INSTANCE_COSTS"); raw != "" {
		var override map[string]float64
		if err := json.Unmarshal([]byte(raw), &override); err == nil {
			for k, v := range override {
				costs[k] = v
			}
		}
	}
	return costs
}

// maxMonthlySpendEUR: the account budget. The founder set it at 20
// (~$20/month, 21/10); Hetzner bills in EUR, so the gate is EUR — 20 EUR is
// the stricter and simpler reading.
func maxMonthlySpendEUR() float64 {
	if v := os.Getenv("MAX_MONTHLY_SPEND_EUR"); v != "" {
		var n float64
		if _, err := fmt.Sscanf(v, "%g", &n); err == nil && n > 0 {
			return n
		}
	}
	return 20
}

// provScanJob is the tolerant shape of a provisioning job file
// (prov_*.json in the credentials dir, written by the webhook trigger —
// provisioning.go names the state field "state" and the type "server_type").
type provScanJob struct {
	State      string `json:"state"`
	ServerType string `json:"server_type"`
}

// monthlyCommittedEUR sums the estimated monthly cost of provisioning jobs
// in queued or running state — the committed spend the next request adds to.
func monthlyCommittedEUR() float64 {
	costs := instanceCosts()
	dir := env("LANDING_CRED_DIR", ".")
	files, err := filepath.Glob(filepath.Join(dir, "prov_*.json"))
	if err != nil {
		return 0
	}
	committed := 0.0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var job provScanJob
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		if job.State != "queued" && job.State != "running" {
			continue
		}
		committed += costs[job.ServerType]
	}
	return committed
}

// spendGuard checks a dedicated provisioning request against the account
// budget: unknown types are rejected (fail-closed — that includes every CCX
// dedicated-tier type), and the committed spend plus the new instance must
// stay under the monthly budget.
func spendGuard(instanceType string) error {
	costs := instanceCosts()
	cost, ok := costs[instanceType]
	if !ok {
		return fmt.Errorf("instance type %q is not in the account budget whitelist", instanceType)
	}
	committed := monthlyCommittedEUR()
	if committed+cost > maxMonthlySpendEUR() {
		return fmt.Errorf("monthly spend limit reached — committed %.2f EUR + %.2f EUR exceeds the %.0f EUR budget", committed, cost, maxMonthlySpendEUR())
	}
	return nil
}
