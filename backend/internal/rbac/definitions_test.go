package rbac

import (
	"slices"
	"testing"
)

var hrDocumentPermissions = []string{
	"hris:employee_identity:view",
	"hris:employee_identity:edit",
	"hris:contract:view",
	"hris:contract:manage",
	"hris:contract:send",
	"hris:payslip:view",
	"hris:payslip:manage",
	"hris:payslip:send",
}

func TestHRDocumentPermissionsAreSensitiveAndAdminOnly(t *testing.T) {
	defined := map[string]PermissionDefinition{}
	for _, permission := range DefaultPermissions() {
		if _, dup := defined[permission.ID]; dup {
			t.Fatalf("permission %s defined twice", permission.ID)
		}
		defined[permission.ID] = permission
	}

	admin := SystemRolePermissionIDs(RoleAdmin)
	for _, id := range hrDocumentPermissions {
		permission, ok := defined[id]
		if !ok {
			t.Fatalf("permission %s is not defined", id)
		}
		if !permission.IsSensitive {
			t.Errorf("%s must be IsSensitive", id)
		}
		if permission.ModuleID != ModuleHRIS {
			t.Errorf("%s module = %s, want hris", id, permission.ModuleID)
		}
		if permission.Description == "" {
			t.Errorf("%s needs a description", id)
		}
		if !slices.Contains(admin, id) {
			t.Errorf("Admin must get %s", id)
		}
		for _, role := range []string{RoleManager, RoleStaff, RoleViewer} {
			if slices.Contains(SystemRolePermissionIDs(role), id) {
				t.Errorf("%s must not get %s", role, id)
			}
		}
	}
}

func TestBaselineV2GrantsHRDocumentPermissions(t *testing.T) {
	if currentBaselineVersion != 2 {
		t.Fatalf("currentBaselineVersion = %d, want 2", currentBaselineVersion)
	}
	got := slices.Clone(baselinePermissionVersions[2])
	want := slices.Clone(hrDocumentPermissions)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("baseline v2 = %v, want %v", got, want)
	}
	for version := 1; version <= currentBaselineVersion; version++ {
		if len(baselinePermissionVersions[version]) == 0 {
			t.Fatalf("baseline version %d is empty", version)
		}
	}
}
