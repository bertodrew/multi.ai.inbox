package core

import (
	"slices"
	"testing"
)

// A full access key (nil permissions) must not be able to erase the tenant:
// data_delete is explicit-only, so an agent holding a full key cannot delete
// the organization it was screened from.
func TestDataDeleteIsNeverImplied(t *testing.T) {
	full := &Principal{}
	if full.Can(PermDataDelete) {
		t.Fatal("a full access key must not hold data_delete")
	}
	if !full.Can(PermDataExport) {
		t.Fatal("a full access key must hold data_export")
	}
	explicit := &Principal{Permissions: []string{PermDataDelete}}
	if !explicit.Can(PermDataDelete) {
		t.Fatal("a key that lists data_delete must hold it")
	}
}

// Both permissions must be grantable, otherwise the operator could never
// issue the key that makes self-service export and erasure possible.
func TestGrantablePermissions(t *testing.T) {
	for _, perm := range []string{PermDataExport, PermDataDelete, PermMessageRelease} {
		if !slices.Contains(GrantablePermissions, perm) {
			t.Fatalf("%s is not grantable", perm)
		}
	}
	seen := map[string]bool{}
	for _, perm := range GrantablePermissions {
		if seen[perm] {
			t.Fatalf("%s appears twice in GrantablePermissions", perm)
		}
		seen[perm] = true
	}
}
