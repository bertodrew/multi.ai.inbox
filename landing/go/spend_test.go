package main

// Tests for the account spend guard (21/10, founder): monthly budget ~$20,
// fail-closed on unknown types (all CCX dedicated-tier types included), and
// honest numbers in the refusal message.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpendGuardAllowsEntryLevel(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	if err := spendGuard("cx33"); err != nil {
		t.Fatalf("entry-level instance must pass the default budget: %v", err)
	}
	if err := spendGuard("cpx12"); err != nil {
		t.Fatalf("cheapest orderable type must pass: %v", err)
	}
}

func TestSpendGuardRejectsUnknownType(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	// every CCX dedicated-tier type is deliberately outside the budget
	for _, ttype := range []string{"ccx41", "ccx42", "ccx43", "ccx13"} {
		err := spendGuard(ttype)
		if err == nil {
			t.Fatalf("dedicated-tier type %q must be rejected under the budget", ttype)
		}
		if !strings.Contains(err.Error(), "whitelist") {
			t.Fatalf("refusal must name the whitelist: %v", err)
		}
	}
}

func TestSpendGuardRejectsOverBudget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LANDING_CRED_DIR", dir)
	// two committed running jobs on cx33 (8.49 each) = 16.98 committed
	for _, id := range []string{"one", "two"} {
		_ = osWriteFile(filepath.Join(dir, "prov_"+id+".json"),
			[]byte(`{"status":"running","server_type":"cx33"}`))
	}
	err := spendGuard("cx33")
	if err == nil {
		t.Fatalf("16.98 committed + 8.49 new must exceed the 20 EUR budget")
	}
	if !strings.Contains(err.Error(), "16.98") || !strings.Contains(err.Error(), "8.49") || !strings.Contains(err.Error(), "20") {
		t.Fatalf("refusal must show honest numbers: %v", err)
	}
	// a single committed job leaves room: 8.49 + 8.49 = 16.98 < 20
	_ = os.Remove(filepath.Join(dir, "prov_two.json"))
	if err := spendGuard("cx33"); err != nil {
		t.Fatalf("8.49 committed + 8.49 new must pass: %v", err)
	}
}

func TestSpendGuardBudgetFromEnv(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	t.Setenv("MAX_MONTHLY_SPEND_EUR", "5")
	if err := spendGuard("cx33"); err == nil {
		t.Fatalf("cx33 (8.49) must exceed a 5 EUR budget")
	}
	if err := spendGuard("cx23"); err == nil {
		t.Fatalf("cx23 (5.49) exceeds a 5 EUR budget")
	}
}

func TestMonthlyCommittedFieldTolerance(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LANDING_CRED_DIR", dir)
	// the webhook trigger may name the field server_type, instance_type or type
	_ = osWriteFile(filepath.Join(dir, "prov_a.json"), []byte(`{"status":"queued","server_type":"cx33"}`))
	_ = osWriteFile(filepath.Join(dir, "prov_b.json"), []byte(`{"status":"running","instance_type":"cpx12"}`))
	_ = osWriteFile(filepath.Join(dir, "prov_c.json"), []byte(`{"status":"running","type":"cx23"}`))
	_ = osWriteFile(filepath.Join(dir, "prov_d.json"), []byte(`{"status":"failed","server_type":"ccx41"}`))
	got := monthlyCommittedEUR()
	// 8.49 + 11.99 + 5.49 = 25.97; failed jobs do not count
	if got < 25.9 || got > 26.0 {
		t.Fatalf("committed sum must count queued+running only, got %.2f", got)
	}
}
