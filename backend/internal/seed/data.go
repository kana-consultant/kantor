// Package seed contains hard-coded demo fixtures used by the cmd/seed CLI.
//
// Demo data is intentionally kept in code (not env vars / YAML) so that
// shipping a release with a misconfigured env var cannot accidentally
// create accounts with known passwords in production. Seeding only ever
// runs through an explicit `go run ./cmd/seed` invocation.
package seed

import "github.com/kana-consultant/kantor/backend/internal/rbac"

// SuperAdmin is the demo super admin account.
type SuperAdmin struct {
	Email    string
	Password string
	FullName string
}

// User is one demo user record with the role assignment to apply.
type User struct {
	Email      string
	Password   string
	FullName   string
	Department string
	Skills     []string
	Roles      []rbac.RoleKey
}

// DemoSuperAdmin is the canonical demo super admin.
var DemoSuperAdmin = SuperAdmin{
	Email:    "superadmin@kantor.local",
	Password: "Password123!",
	FullName: "Seeded Super Admin",
}

// DemoUsers is the list of non-admin demo accounts created by the seed CLI.
var DemoUsers = []User{
	{
		Email:      "staff.ops@kantor.local",
		Password:   "Password123!",
		FullName:   "Operational Staff",
		Department: "engineering",
		Skills:     []string{"frontend", "kanban"},
		Roles:      []rbac.RoleKey{{Name: "staff", Module: "operational"}},
	},
	{
		Email:      "viewer.ops@kantor.local",
		Password:   "Password123!",
		FullName:   "Operational Viewer",
		Department: "finance",
		Skills:     []string{"qa", "reporting"},
		Roles:      []rbac.RoleKey{{Name: "viewer", Module: "operational"}},
	},
	{
		Email:      "staff.marketing@kantor.local",
		Password:   "Password123!",
		FullName:   "Marketing Staff",
		Department: "marketing",
		Skills:     []string{"copywriting", "ads", "crm"},
		Roles:      []rbac.RoleKey{{Name: "staff", Module: "marketing"}},
	},
	{
		Email:      "viewer.marketing@kantor.local",
		Password:   "Password123!",
		FullName:   "Marketing Viewer",
		Department: "marketing",
		Skills:     []string{"reporting"},
		Roles:      []rbac.RoleKey{{Name: "viewer", Module: "marketing"}},
	},
}
