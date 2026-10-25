package domain

import "testing"

func TestValidPermission(t *testing.T) {
	for _, p := range AllBotPermissions {
		if !ValidPermission(p) {
			t.Fatalf("expected %q to be valid", p)
		}
	}
	for _, bad := range []string{"", "bot:admin", "BOT:READ", "super", "bot:read "} {
		if ValidPermission(bad) {
			t.Fatalf("expected %q to be invalid", bad)
		}
	}
}

func TestAdminIsSuperAdmin(t *testing.T) {
	if !(&Admin{Role: RoleSuperAdmin}).IsSuperAdmin() {
		t.Fatal("superadmin role must report true")
	}
	if (&Admin{Role: RoleAdmin}).IsSuperAdmin() {
		t.Fatal("admin role must report false")
	}
}
