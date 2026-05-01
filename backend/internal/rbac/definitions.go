package rbac

import "sort"

type RoleKey struct {
	Name   string
	Module string
}

type ModuleDefinition struct {
	ID           string
	Name         string
	Description  string
	DisplayOrder int
}

type PermissionDefinition struct {
	ID          string
	ModuleID    string
	Resource    string
	Action      string
	Description string
	IsSensitive bool
}

type RoleDefinition struct {
	Name           string
	Slug           string
	Description    string
	IsSystem       bool
	HierarchyLevel int
}

const (
	ModuleOperational = "operational"
	ModuleHRIS        = "hris"
	ModuleMarketing   = "marketing"
	ModuleAdmin       = "admin"

	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleManager    = "manager"
	RoleStaff      = "staff"
	RoleViewer     = "viewer"
)

func Modules() []ModuleDefinition {
	return []ModuleDefinition{
		{
			ID:           ModuleOperational,
			Name:         "Operasional",
			Description:  "Manajemen project, task, kanban, automation, activity tracker, WA broadcast",
			DisplayOrder: 1,
		},
		{
			ID:           ModuleHRIS,
			Name:         "HRIS",
			Description:  "Data karyawan, keuangan, reimbursement, subscription",
			DisplayOrder: 2,
		},
		{
			ID:           ModuleMarketing,
			Name:         "Marketing",
			Description:  "Campaign, ads metrics, leads management",
			DisplayOrder: 3,
		},
		{
			ID:           ModuleAdmin,
			Name:         "Admin",
			Description:  "Audit log, role management, user management, system settings",
			DisplayOrder: 4,
		},
	}
}

func SystemRoles() []RoleDefinition {
	return []RoleDefinition{
		{
			Name:           "Super Admin",
			Slug:           RoleSuperAdmin,
			Description:    "Akses penuh ke seluruh sistem tanpa assignment modul",
			IsSystem:       true,
			HierarchyLevel: 100,
		},
		{
			Name:           "Admin",
			Slug:           RoleAdmin,
			Description:    "Akses penuh di modul yang di-assign",
			IsSystem:       true,
			HierarchyLevel: 80,
		},
		{
			Name:           "Manager",
			Slug:           RoleManager,
			Description:    "Akses managerial di modul yang di-assign",
			IsSystem:       true,
			HierarchyLevel: 60,
		},
		{
			Name:           "Staff",
			Slug:           RoleStaff,
			Description:    "Akses operasional harian di modul yang di-assign",
			IsSystem:       true,
			HierarchyLevel: 40,
		},
		{
			Name:           "Viewer",
			Slug:           RoleViewer,
			Description:    "Akses baca di modul yang di-assign",
			IsSystem:       true,
			HierarchyLevel: 20,
		},
	}
}

func ReservedRoleSlugs() []string {
	return []string{
		RoleSuperAdmin,
		RoleAdmin,
		RoleManager,
		RoleStaff,
		RoleViewer,
	}
}

func DefaultPermissions() []PermissionDefinition {
	return []PermissionDefinition{
		// Operational
		{ID: "operational:project:view", ModuleID: ModuleOperational, Resource: "project", Action: "view", Description: "Melihat daftar dan detail project"},
		{ID: "operational:project:create", ModuleID: ModuleOperational, Resource: "project", Action: "create", Description: "Membuat project baru"},
		{ID: "operational:project:edit", ModuleID: ModuleOperational, Resource: "project", Action: "edit", Description: "Mengedit project"},
		{ID: "operational:project:delete", ModuleID: ModuleOperational, Resource: "project", Action: "delete", Description: "Menghapus project"},
		{ID: "operational:project:manage_members", ModuleID: ModuleOperational, Resource: "project", Action: "manage_members", Description: "Assign dan remove anggota project"},
		{ID: "operational:task:view", ModuleID: ModuleOperational, Resource: "task", Action: "view", Description: "Melihat task di kanban board"},
		{ID: "operational:task:create", ModuleID: ModuleOperational, Resource: "task", Action: "create", Description: "Membuat task baru"},
		{ID: "operational:task:edit", ModuleID: ModuleOperational, Resource: "task", Action: "edit", Description: "Mengedit task"},
		{ID: "operational:task:delete", ModuleID: ModuleOperational, Resource: "task", Action: "delete", Description: "Menghapus task"},
		{ID: "operational:task:assign", ModuleID: ModuleOperational, Resource: "task", Action: "assign", Description: "Assign task ke user lain"},
		{ID: "operational:column:view", ModuleID: ModuleOperational, Resource: "column", Action: "view", Description: "Melihat kolom kanban"},
		{ID: "operational:column:manage", ModuleID: ModuleOperational, Resource: "column", Action: "manage", Description: "Tambah edit hapus dan reorder kolom kanban"},
		{ID: "operational:assignment_rule:view", ModuleID: ModuleOperational, Resource: "assignment_rule", Action: "view", Description: "Melihat assignment rules"},
		{ID: "operational:assignment_rule:manage", ModuleID: ModuleOperational, Resource: "assignment_rule", Action: "manage", Description: "Mengelola assignment rules"},
		{ID: "operational:tracker:view", ModuleID: ModuleOperational, Resource: "tracker", Action: "view", Description: "Melihat aktivitas tracker sendiri"},
		{ID: "operational:tracker:view_team", ModuleID: ModuleOperational, Resource: "tracker", Action: "view_team", Description: "Melihat aktivitas tracker seluruh tim", IsSensitive: true},
		{ID: "operational:tracker:manage_domains", ModuleID: ModuleOperational, Resource: "tracker", Action: "manage_domains", Description: "Mengelola domain categories tracker"},
		{ID: "operational:tracker-reminder:manage", ModuleID: ModuleOperational, Resource: "tracker_reminder", Action: "manage", Description: "Mengelola pengaturan reminder activity tracker"},
		{ID: "operational:wa:view", ModuleID: ModuleOperational, Resource: "wa_broadcast", Action: "view", Description: "Melihat dashboard dan log WA broadcast"},
		{ID: "operational:wa:manage", ModuleID: ModuleOperational, Resource: "wa_broadcast", Action: "manage", Description: "Mengelola template schedule dan pengiriman WA"},
		{ID: "operational:vps:view", ModuleID: ModuleOperational, Resource: "vps", Action: "view", Description: "Melihat inventaris VPS dan status uptime", IsSensitive: true},
		{ID: "operational:vps:create", ModuleID: ModuleOperational, Resource: "vps", Action: "create", Description: "Mendaftarkan VPS baru ke inventaris", IsSensitive: true},
		{ID: "operational:vps:edit", ModuleID: ModuleOperational, Resource: "vps", Action: "edit", Description: "Mengedit VPS, app, dan health check", IsSensitive: true},
		{ID: "operational:vps:delete", ModuleID: ModuleOperational, Resource: "vps", Action: "delete", Description: "Menghapus VPS dari inventaris", IsSensitive: true},

		// HRIS
		{ID: "hris:employee:view", ModuleID: ModuleHRIS, Resource: "employee", Action: "view", Description: "Melihat data karyawan"},
		{ID: "hris:employee:create", ModuleID: ModuleHRIS, Resource: "employee", Action: "create", Description: "Menambah karyawan baru"},
		{ID: "hris:employee:edit", ModuleID: ModuleHRIS, Resource: "employee", Action: "edit", Description: "Mengedit data karyawan"},
		{ID: "hris:employee:delete", ModuleID: ModuleHRIS, Resource: "employee", Action: "delete", Description: "Menghapus karyawan"},
		{ID: "hris:department:view", ModuleID: ModuleHRIS, Resource: "department", Action: "view", Description: "Melihat departemen"},
		{ID: "hris:department:create", ModuleID: ModuleHRIS, Resource: "department", Action: "create", Description: "Membuat departemen"},
		{ID: "hris:department:edit", ModuleID: ModuleHRIS, Resource: "department", Action: "edit", Description: "Mengedit departemen"},
		{ID: "hris:department:delete", ModuleID: ModuleHRIS, Resource: "department", Action: "delete", Description: "Menghapus departemen"},
		{ID: "hris:salary:view", ModuleID: ModuleHRIS, Resource: "salary", Action: "view", Description: "Melihat data gaji karyawan", IsSensitive: true},
		{ID: "hris:salary:create", ModuleID: ModuleHRIS, Resource: "salary", Action: "create", Description: "Menginput data gaji", IsSensitive: true},
		{ID: "hris:bonus:view", ModuleID: ModuleHRIS, Resource: "bonus", Action: "view", Description: "Melihat data bonus", IsSensitive: true},
		{ID: "hris:bonus:create", ModuleID: ModuleHRIS, Resource: "bonus", Action: "create", Description: "Menginput bonus", IsSensitive: true},
		{ID: "hris:bonus:edit", ModuleID: ModuleHRIS, Resource: "bonus", Action: "edit", Description: "Mengedit bonus pending", IsSensitive: true},
		{ID: "hris:bonus:delete", ModuleID: ModuleHRIS, Resource: "bonus", Action: "delete", Description: "Menghapus bonus pending", IsSensitive: true},
		{ID: "hris:bonus:approve", ModuleID: ModuleHRIS, Resource: "bonus", Action: "approve", Description: "Approve atau reject bonus", IsSensitive: true},
		{ID: "hris:subscription:view", ModuleID: ModuleHRIS, Resource: "subscription", Action: "view", Description: "Melihat subscription"},
		{ID: "hris:subscription:create", ModuleID: ModuleHRIS, Resource: "subscription", Action: "create", Description: "Menambah subscription"},
		{ID: "hris:subscription:edit", ModuleID: ModuleHRIS, Resource: "subscription", Action: "edit", Description: "Mengedit subscription"},
		{ID: "hris:subscription:delete", ModuleID: ModuleHRIS, Resource: "subscription", Action: "delete", Description: "Menghapus subscription"},
		{ID: "hris:finance:view", ModuleID: ModuleHRIS, Resource: "finance", Action: "view", Description: "Melihat data keuangan"},
		{ID: "hris:finance:create", ModuleID: ModuleHRIS, Resource: "finance", Action: "create", Description: "Menginput record keuangan"},
		{ID: "hris:finance:edit", ModuleID: ModuleHRIS, Resource: "finance", Action: "edit", Description: "Mengedit record keuangan"},
		{ID: "hris:finance:delete", ModuleID: ModuleHRIS, Resource: "finance", Action: "delete", Description: "Menghapus record keuangan"},
		{ID: "hris:finance:approve", ModuleID: ModuleHRIS, Resource: "finance", Action: "approve", Description: "Approve atau reject record keuangan"},
		{ID: "hris:reimbursement:view", ModuleID: ModuleHRIS, Resource: "reimbursement", Action: "view", Description: "Melihat reimbursement milik sendiri"},
		{ID: "hris:reimbursement:view_all", ModuleID: ModuleHRIS, Resource: "reimbursement", Action: "view_all", Description: "Melihat semua reimbursement", IsSensitive: true},
		{ID: "hris:reimbursement:create", ModuleID: ModuleHRIS, Resource: "reimbursement", Action: "create", Description: "Mengajukan reimbursement"},
		{ID: "hris:reimbursement:edit", ModuleID: ModuleHRIS, Resource: "reimbursement", Action: "edit", Description: "Mengedit reimbursement dan upload attachment"},
		{ID: "hris:reimbursement:approve", ModuleID: ModuleHRIS, Resource: "reimbursement", Action: "approve", Description: "Approve atau reject reimbursement"},
		{ID: "hris:reimbursement:mark_paid", ModuleID: ModuleHRIS, Resource: "reimbursement", Action: "mark_paid", Description: "Menandai reimbursement sudah dibayar", IsSensitive: true},

		// Marketing
		{ID: "marketing:campaign:view", ModuleID: ModuleMarketing, Resource: "campaign", Action: "view", Description: "Melihat campaign"},
		{ID: "marketing:campaign:create", ModuleID: ModuleMarketing, Resource: "campaign", Action: "create", Description: "Membuat campaign"},
		{ID: "marketing:campaign:edit", ModuleID: ModuleMarketing, Resource: "campaign", Action: "edit", Description: "Mengedit campaign"},
		{ID: "marketing:campaign:delete", ModuleID: ModuleMarketing, Resource: "campaign", Action: "delete", Description: "Menghapus campaign"},
		{ID: "marketing:campaign:manage_columns", ModuleID: ModuleMarketing, Resource: "campaign", Action: "manage_columns", Description: "Mengelola kolom kanban campaign"},
		{ID: "marketing:ads_metrics:view", ModuleID: ModuleMarketing, Resource: "ads_metrics", Action: "view", Description: "Melihat data ads metrics"},
		{ID: "marketing:ads_metrics:create", ModuleID: ModuleMarketing, Resource: "ads_metrics", Action: "create", Description: "Menginput ads metrics"},
		{ID: "marketing:ads_metrics:edit", ModuleID: ModuleMarketing, Resource: "ads_metrics", Action: "edit", Description: "Mengedit ads metrics"},
		{ID: "marketing:ads_metrics:delete", ModuleID: ModuleMarketing, Resource: "ads_metrics", Action: "delete", Description: "Menghapus ads metrics"},
		{ID: "marketing:leads:view", ModuleID: ModuleMarketing, Resource: "leads", Action: "view", Description: "Melihat leads"},
		{ID: "marketing:leads:create", ModuleID: ModuleMarketing, Resource: "leads", Action: "create", Description: "Menambah lead"},
		{ID: "marketing:leads:edit", ModuleID: ModuleMarketing, Resource: "leads", Action: "edit", Description: "Mengedit lead"},
		{ID: "marketing:leads:delete", ModuleID: ModuleMarketing, Resource: "leads", Action: "delete", Description: "Menghapus lead"},
		{ID: "marketing:leads:import", ModuleID: ModuleMarketing, Resource: "leads", Action: "import", Description: "Bulk import leads dari CSV"},

		// Admin
		{ID: "admin:audit_log:view", ModuleID: ModuleAdmin, Resource: "audit_log", Action: "view", Description: "Melihat audit log", IsSensitive: true},
		{ID: "admin:audit_log:export", ModuleID: ModuleAdmin, Resource: "audit_log", Action: "export", Description: "Export audit log", IsSensitive: true},
		{ID: "admin:roles:view", ModuleID: ModuleAdmin, Resource: "roles", Action: "view", Description: "Melihat daftar roles"},
		{ID: "admin:roles:manage", ModuleID: ModuleAdmin, Resource: "roles", Action: "manage", Description: "Membuat mengedit menghapus roles dan permissions", IsSensitive: true},
		{ID: "admin:users:view", ModuleID: ModuleAdmin, Resource: "users", Action: "view", Description: "Melihat daftar users"},
		{ID: "admin:users:manage", ModuleID: ModuleAdmin, Resource: "users", Action: "manage", Description: "Assign role dan manage user access", IsSensitive: true},
		{ID: "admin:settings:view", ModuleID: ModuleAdmin, Resource: "settings", Action: "view", Description: "Melihat system settings"},
		{ID: "admin:settings:manage", ModuleID: ModuleAdmin, Resource: "settings", Action: "manage", Description: "Mengubah system settings", IsSensitive: true},
	}
}

func SystemRolePermissionIDs(roleSlug string) []string {
	allPermissions := DefaultPermissions()
	result := make([]string, 0, len(allPermissions))

	for _, permission := range allPermissions {
		switch roleSlug {
		case RoleAdmin:
			result = append(result, permission.ID)
		case RoleManager:
			if managerCanAccess(permission) {
				result = append(result, permission.ID)
			}
		case RoleStaff:
			if staffCanAccess(permission) {
				result = append(result, permission.ID)
			}
		case RoleViewer:
			if viewerCanAccess(permission) {
				result = append(result, permission.ID)
			}
		}
	}

	sort.Strings(result)
	return result
}

func managerCanAccess(permission PermissionDefinition) bool {
	if permission.ModuleID == ModuleAdmin {
		switch permission.ID {
		case "admin:roles:manage", "admin:users:manage", "admin:settings:manage":
			return false
		default:
			return permission.Action == "view"
		}
	}

	if permission.Action == "delete" {
		return false
	}

	if permission.ID == "marketing:campaign:manage_columns" || permission.ID == "operational:column:manage" {
		return false
	}

	if permission.ID == "hris:reimbursement:mark_paid" {
		return false
	}

	if permission.ID == "operational:tracker-reminder:manage" {
		return false
	}

	// VPS monitoring is restricted to super_admin + admin by default.
	if permission.ModuleID == ModuleOperational && permission.Resource == "vps" {
		return false
	}

	return true
}

func staffCanAccess(permission PermissionDefinition) bool {
	if permission.ModuleID == ModuleAdmin {
		return permission.Action == "view"
	}

	if permission.IsSensitive {
		return false
	}

	switch permission.Action {
	case "view", "create", "edit":
		return true
	case "assign":
		return permission.ID == "operational:task:assign"
	default:
		return false
	}
}

func viewerCanAccess(permission PermissionDefinition) bool {
	if permission.IsSensitive {
		return false
	}

	return permission.Action == "view"
}
