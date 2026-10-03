package hris

import (
	"testing"

	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
)

func TestCanSeeFullBankAccount(t *testing.T) {
	owner := "user-1"
	employee := model.Employee{UserID: &owner}
	cases := []struct {
		name      string
		principal platformmiddleware.Principal
		want      bool
	}{
		{"no identity permission", platformmiddleware.Principal{UserID: "user-2", Cached: &rbac.CachedPermissions{Permissions: map[string]bool{"hris:employee:view": true}}}, false},
		{"identity view", platformmiddleware.Principal{UserID: "user-2", Cached: &rbac.CachedPermissions{Permissions: map[string]bool{permissionIdentityView: true}}}, true},
		{"owner", platformmiddleware.Principal{UserID: owner}, true},
		{"super admin", platformmiddleware.Principal{UserID: "user-3", IsSuperAdmin: true}, true},
		{"pat permission list", platformmiddleware.Principal{UserID: "user-4", Permissions: []string{permissionIdentityView}}, true},
	}
	for _, tc := range cases {
		if got := canSeeFullBankAccount(tc.principal, employee); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if canSeeFullBankAccount(platformmiddleware.Principal{}, model.Employee{}) {
		t.Error("an unlinked record must not count as owned by an empty user id")
	}
}

func TestMaskedAccountForAudit(t *testing.T) {
	full := "1234567890"
	if got := maskedAccountForAudit(&full); got == nil || *got != "******7890" {
		t.Fatalf("masked = %v", got)
	}
	mask := "******7890"
	if got := maskedAccountForAudit(&mask); *got != mask {
		t.Fatalf("an echoed mask stays as is, got %v", *got)
	}
	if maskedAccountForAudit(nil) != nil {
		t.Fatal("nil stays nil")
	}
}
