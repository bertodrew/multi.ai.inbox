package core

import (
	"slices"
	"testing"
)

func has(perms []string, want string) bool { return slices.Contains(perms, want) }

// The owner answers for the tenant, so it holds the explicit-only permissions;
// an admin manages everything else but cannot erase the organization. A member
// works with mail and keys, and cannot manage domains, keys or billing.
func TestPermissionsForRole(t *testing.T) {
	owner, admin, member := PermissionsForRole(RoleOwner), PermissionsForRole(RoleAdmin), PermissionsForRole(RoleMember)

	for _, perm := range append(append([]string{}, AllPermissions...), ExplicitPermissions...) {
		if !has(owner, perm) {
			t.Errorf("owner lacks %s", perm)
		}
	}
	if !has(admin, PermMessageRelease) {
		t.Error("admin must be able to release held mail")
	}
	if has(admin, PermDataDelete) {
		t.Error("only the owner may delete the organization")
	}
	if has(member, PermDataDelete) || has(member, PermMessageRelease) {
		t.Error("a member holds no explicit permission")
	}
	for _, perm := range []string{PermInboxRead, PermMessageRead, PermMessageSend, PermMessageUpdate, PermBillingRead} {
		if !has(member, perm) {
			t.Errorf("member lacks %s", perm)
		}
	}
	for _, perm := range []string{PermDomainWrite, PermAPIKeyWrite, PermBillingWrite, PermWebhookWrite} {
		if has(member, perm) {
			t.Errorf("member must not hold %s", perm)
		}
	}
}

// Every permission a role grants must be one the API knows and a key could
// hold, otherwise the role is a typo away from a lockout.
func TestRolePermissionsAreGrantable(t *testing.T) {
	for _, role := range []string{RoleOwner, RoleAdmin, RoleMember, "", "unknown"} {
		perms := PermissionsForRole(role)
		if len(perms) == 0 {
			t.Fatalf("%q grants nothing", role)
		}
		if !slices.IsSorted(perms) {
			t.Fatalf("%q: permissions are not sorted", role)
		}
		for i, perm := range perms {
			if !slices.Contains(GrantablePermissions, perm) {
				t.Fatalf("%q grants unknown permission %q", role, perm)
			}
			if i > 0 && perms[i-1] == perm {
				t.Fatalf("%q repeats %q", role, perm)
			}
		}
	}
}

func TestValidRole(t *testing.T) {
	for _, role := range []string{RoleOwner, RoleAdmin, RoleMember} {
		if !ValidRole(role) {
			t.Fatalf("%q must be valid", role)
		}
	}
	for _, role := range []string{"", "Owner", "root", "admin "} {
		if ValidRole(role) {
			t.Fatalf("%q must not be valid", role)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		" Person@Example.COM ":      "person@example.com",
		"a.b+tag@sub.example.co.uk": "a.b+tag@sub.example.co.uk",
	}
	for in, want := range cases {
		got, err := NormalizeEmail(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got != want {
			t.Fatalf("%q normalized to %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"", "x", "no-at-sign", "a@b", "a@.b", "a@b.", "two words@example.com", "@example.com"} {
		if _, err := NormalizeEmail(in); err == nil {
			t.Fatalf("%q accepted", in)
		}
	}
}

func TestSafeRedirect(t *testing.T) {
	for _, in := range []string{"/login/", "/console/inbox?x=1"} {
		if got := SafeRedirect(in); got != in {
			t.Fatalf("%q became %q", in, got)
		}
	}
	for _, in := range []string{"", "//evil.example", "https://evil.example", "login/", "/a\\b", "/a\nb"} {
		if got := SafeRedirect(in); got != "" {
			t.Fatalf("%q accepted as %q", in, got)
		}
	}
}
