package rbac

type RoleKey struct {
	Name   string
	Module string
}

type RoleDefinition struct {
	Name        string
	Module      string
	Description string
}

type PermissionDefinition struct {
	Name        string
	Module      string
	Resource    string
	Action      string
	Description string
}

var modules = []string{"operational", "hris", "marketing"}

var defaultPermissions = []PermissionDefinition{
	{Name: "operational:project:view", Module: "operational", Resource: "project", Action: "view", Description: "View operational projects"},
	{Name: "operational:project:create", Module: "operational", Resource: "project", Action: "create", Description: "Create operational projects"},
	{Name: "operational:project:edit", Module: "operational", Resource: "project", Action: "edit", Description: "Edit operational projects"},
	{Name: "operational:project:delete", Module: "operational", Resource: "project", Action: "delete", Description: "Delete operational projects"},
	{Name: "operational:kanban:view", Module: "operational", Resource: "kanban", Action: "view", Description: "View operational kanban board"},
	{Name: "operational:kanban:create", Module: "operational", Resource: "kanban", Action: "create", Description: "Create operational kanban columns and tasks"},
	{Name: "operational:kanban:edit", Module: "operational", Resource: "kanban", Action: "edit", Description: "Edit operational kanban columns and tasks"},
	{Name: "operational:kanban:delete", Module: "operational", Resource: "kanban", Action: "delete", Description: "Delete operational kanban columns and tasks"},
	{Name: "operational:task:view", Module: "operational", Resource: "task", Action: "view", Description: "View operational tasks"},
	{Name: "operational:task:create", Module: "operational", Resource: "task", Action: "create", Description: "Create operational tasks"},
	{Name: "operational:task:edit", Module: "operational", Resource: "task", Action: "edit", Description: "Edit operational tasks"},
	{Name: "operational:task:delete", Module: "operational", Resource: "task", Action: "delete", Description: "Delete operational tasks"},
	{Name: "operational:tracker:view", Module: "operational", Resource: "tracker", Action: "view", Description: "View personal activity tracker data"},
	{Name: "operational:tracker:view_team", Module: "operational", Resource: "tracker", Action: "view_team", Description: "View team activity tracker data"},
	{Name: "operational:tracker_consent:audit", Module: "operational", Resource: "tracker_consent", Action: "audit", Description: "View tracker consent audit report"},
	{Name: "operational:tracker_domain:manage", Module: "operational", Resource: "tracker_domain", Action: "manage", Description: "Manage activity tracker domain categories"},
	{Name: "hris:employee:view", Module: "hris", Resource: "employee", Action: "view", Description: "View employee data"},
	{Name: "hris:employee:create", Module: "hris", Resource: "employee", Action: "create", Description: "Create employee data"},
	{Name: "hris:employee:edit", Module: "hris", Resource: "employee", Action: "edit", Description: "Edit employee data"},
	{Name: "hris:employee:delete", Module: "hris", Resource: "employee", Action: "delete", Description: "Delete employee data"},
	{Name: "hris:department:view", Module: "hris", Resource: "department", Action: "view", Description: "View departments"},
	{Name: "hris:department:create", Module: "hris", Resource: "department", Action: "create", Description: "Create departments"},
	{Name: "hris:department:edit", Module: "hris", Resource: "department", Action: "edit", Description: "Edit departments"},
	{Name: "hris:department:delete", Module: "hris", Resource: "department", Action: "delete", Description: "Delete departments"},
	{Name: "hris:salary:view", Module: "hris", Resource: "salary", Action: "view", Description: "View salary data"},
	{Name: "hris:salary:create", Module: "hris", Resource: "salary", Action: "create", Description: "Create salary records"},
	{Name: "hris:salary:edit", Module: "hris", Resource: "salary", Action: "edit", Description: "Edit salary data"},
	{Name: "hris:bonus:view", Module: "hris", Resource: "bonus", Action: "view", Description: "View bonus data"},
	{Name: "hris:bonus:create", Module: "hris", Resource: "bonus", Action: "create", Description: "Create bonus entries"},
	{Name: "hris:bonus:edit", Module: "hris", Resource: "bonus", Action: "edit", Description: "Edit bonus entries"},
	{Name: "hris:bonus:approve", Module: "hris", Resource: "bonus", Action: "approve", Description: "Approve bonus entries"},
	{Name: "hris:subscription:view", Module: "hris", Resource: "subscription", Action: "view", Description: "View subscriptions"},
	{Name: "hris:subscription:create", Module: "hris", Resource: "subscription", Action: "create", Description: "Create subscriptions"},
	{Name: "hris:subscription:edit", Module: "hris", Resource: "subscription", Action: "edit", Description: "Edit subscriptions"},
	{Name: "hris:subscription:delete", Module: "hris", Resource: "subscription", Action: "delete", Description: "Delete subscriptions"},
	{Name: "hris:finance:view", Module: "hris", Resource: "finance", Action: "view", Description: "View finance entries"},
	{Name: "hris:finance:create", Module: "hris", Resource: "finance", Action: "create", Description: "Create finance entries"},
	{Name: "hris:finance:edit", Module: "hris", Resource: "finance", Action: "edit", Description: "Edit finance entries"},
	{Name: "hris:finance:approve", Module: "hris", Resource: "finance", Action: "approve", Description: "Approve finance entries"},
	{Name: "hris:reimbursement:view", Module: "hris", Resource: "reimbursement", Action: "view", Description: "View reimbursements"},
	{Name: "hris:reimbursement:create", Module: "hris", Resource: "reimbursement", Action: "create", Description: "Create reimbursements"},
	{Name: "hris:reimbursement:edit", Module: "hris", Resource: "reimbursement", Action: "edit", Description: "Edit reimbursements"},
	{Name: "hris:reimbursement:approve", Module: "hris", Resource: "reimbursement", Action: "approve", Description: "Approve reimbursements"},
	{Name: "marketing:campaign:view", Module: "marketing", Resource: "campaign", Action: "view", Description: "View campaigns"},
	{Name: "marketing:campaign:create", Module: "marketing", Resource: "campaign", Action: "create", Description: "Create campaigns"},
	{Name: "marketing:campaign:edit", Module: "marketing", Resource: "campaign", Action: "edit", Description: "Edit campaigns"},
	{Name: "marketing:campaign:delete", Module: "marketing", Resource: "campaign", Action: "delete", Description: "Delete campaigns"},
	{Name: "marketing:column:manage", Module: "marketing", Resource: "column", Action: "manage", Description: "Manage marketing campaign columns"},
	{Name: "marketing:ads_metrics:view", Module: "marketing", Resource: "ads_metrics", Action: "view", Description: "View ads metrics"},
	{Name: "marketing:ads_metrics:create", Module: "marketing", Resource: "ads_metrics", Action: "create", Description: "Create ads metrics"},
	{Name: "marketing:ads_metrics:edit", Module: "marketing", Resource: "ads_metrics", Action: "edit", Description: "Edit ads metrics"},
	{Name: "marketing:ads_metrics:delete", Module: "marketing", Resource: "ads_metrics", Action: "delete", Description: "Delete ads metrics"},
	{Name: "marketing:metric:view", Module: "marketing", Resource: "metric", Action: "view", Description: "View ad metrics"},
	{Name: "marketing:metric:create", Module: "marketing", Resource: "metric", Action: "create", Description: "Create ad metrics"},
	{Name: "marketing:metric:edit", Module: "marketing", Resource: "metric", Action: "edit", Description: "Edit ad metrics"},
	{Name: "marketing:metric:delete", Module: "marketing", Resource: "metric", Action: "delete", Description: "Delete ad metrics"},
	{Name: "marketing:leads:view", Module: "marketing", Resource: "leads", Action: "view", Description: "View leads"},
	{Name: "marketing:leads:create", Module: "marketing", Resource: "leads", Action: "create", Description: "Create leads"},
	{Name: "marketing:leads:edit", Module: "marketing", Resource: "leads", Action: "edit", Description: "Edit leads"},
	{Name: "marketing:leads:delete", Module: "marketing", Resource: "leads", Action: "delete", Description: "Delete leads"},
	{Name: "operational:wa:view", Module: "operational", Resource: "wa", Action: "view", Description: "View WA broadcast dashboard and logs"},
	{Name: "operational:wa:manage", Module: "operational", Resource: "wa", Action: "manage", Description: "Manage WA broadcast templates, schedules, and send messages"},
}

func DefaultRoles() []RoleDefinition {
	roles := []RoleDefinition{
		{
			Name:        "super_admin",
			Description: "Global full access to all modules and settings",
		},
	}

	for _, module := range modules {
		roles = append(roles,
			RoleDefinition{Name: "admin", Module: module, Description: "Module administrator with full CRUD access"},
			RoleDefinition{Name: "manager", Module: module, Description: "Module manager with view and approval access"},
			RoleDefinition{Name: "staff", Module: module, Description: "Module staff with limited self-service CRUD access"},
			RoleDefinition{Name: "viewer", Module: module, Description: "Module viewer with read-only access"},
		)
	}

	return roles
}

func DefaultPermissions() []PermissionDefinition {
	return defaultPermissions
}

func DefaultRolesForNewUser(existingUsers int64) []RoleKey {
	if existingUsers == 0 {
		return []RoleKey{{Name: "super_admin"}}
	}

	return []RoleKey{
		{Name: "viewer", Module: "operational"},
		{Name: "viewer", Module: "hris"},
		{Name: "viewer", Module: "marketing"},
	}
}

func PermissionNamesForRole(role RoleDefinition) []string {
	if role.Name == "super_admin" {
		names := make([]string, 0, len(defaultPermissions))
		for _, permission := range defaultPermissions {
			names = append(names, permission.Name)
		}

		return names
	}

	modulePermissions := make([]PermissionDefinition, 0)
	for _, permission := range defaultPermissions {
		if permission.Module == role.Module {
			modulePermissions = append(modulePermissions, permission)
		}
	}

	names := make([]string, 0)
	for _, permission := range modulePermissions {
		switch role.Name {
		case "admin":
			names = append(names, permission.Name)
		case "manager":
			if permission.Action == "view" || permission.Action == "approve" || permission.Action == "view_team" || managerCanManage(permission) || managerCanEdit(permission) || managerCanCreate(permission) {
				names = append(names, permission.Name)
			}
		case "staff":
			if staffCanAccess(permission) {
				names = append(names, permission.Name)
			}
		case "viewer":
			if viewerCanAccess(permission) {
				names = append(names, permission.Name)
			}
		}
	}

	return names
}

func managerCanManage(permission PermissionDefinition) bool {
	if permission.Action != "manage" {
		return false
	}

	return permission.Module == "operational" && permission.Resource == "wa"
}

func managerCanEdit(permission PermissionDefinition) bool {
	if permission.Action != "edit" {
		return false
	}

	if permission.Module == "hris" && permission.Resource == "salary" {
		return true
	}

	return true
}

func managerCanCreate(permission PermissionDefinition) bool {
	if permission.Action != "create" {
		return false
	}

	if permission.Module == "hris" && (permission.Resource == "salary" || permission.Resource == "bonus" || permission.Resource == "reimbursement") {
		return true
	}

	return false
}

func staffCanAccess(permission PermissionDefinition) bool {
	// Staff cannot access salary or bonus
	if permission.Module == "hris" && (permission.Resource == "salary" || permission.Resource == "bonus") {
		return false
	}

	// Staff cannot access finance (admin/manager only)
	if permission.Module == "hris" && permission.Resource == "finance" {
		return false
	}

	// Staff cannot manage WA broadcast (admin/manager only)
	if permission.Resource == "wa" && permission.Action == "manage" {
		return false
	}

	switch permission.Action {
	case "view", "create", "edit":
		return true
	default:
		return false
	}
}

func viewerCanAccess(permission PermissionDefinition) bool {
	// Viewers cannot access salary, bonus, or finance at all
	if permission.Module == "hris" && (permission.Resource == "salary" || permission.Resource == "bonus" || permission.Resource == "finance") {
		return false
	}

	// Viewers cannot access WA broadcast
	if permission.Resource == "wa" {
		return false
	}

	// Viewers cannot access subscriptions
	if permission.Module == "hris" && permission.Resource == "subscription" {
		return false
	}

	// Viewers can view and create reimbursements (self-service request)
	if permission.Module == "hris" && permission.Resource == "reimbursement" {
		return permission.Action == "view" || permission.Action == "create"
	}

	return permission.Action == "view"
}
