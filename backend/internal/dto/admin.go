package dto

type ListUsersQuery struct {
	Page       int    `validate:"omitempty,min=1"`
	PerPage    int    `validate:"omitempty,min=1,max=100"`
	Search     string `validate:"omitempty,max=150"`
	ModuleID   string `validate:"omitempty,max=50"`
	RoleID     string `validate:"omitempty,max=36"`
	SuperAdmin *bool  `validate:"omitempty"`
}

type ListRolesQuery struct {
	Search   string `validate:"omitempty,max=150"`
	IsSystem *bool  `validate:"omitempty"`
	IsActive *bool  `validate:"omitempty"`
}

type SetUserModuleRoleRequest struct {
	ModuleID string  `json:"module_id" validate:"required,max=50"`
	RoleID   *string `json:"role_id"`
}

type UpdateUserModuleRolesRequest struct {
	ModuleRoles []SetUserModuleRoleRequest `json:"module_roles" validate:"required,min=1,dive"`
}

type ToggleActiveRequest struct {
	Active bool `json:"active"`
}

type ToggleSuperAdminRequest struct {
	Enabled bool `json:"enabled"`
}

type UpsertRoleRequest struct {
	Name           string   `json:"name" validate:"required,min=3,max=100"`
	Slug           string   `json:"slug" validate:"required,min=3,max=50"`
	Description    string   `json:"description" validate:"omitempty,max=500"`
	HierarchyLevel int      `json:"hierarchy_level" validate:"omitempty,min=1,max=100"`
	PermissionIDs  []string `json:"permission_ids" validate:"required"`
}

type UpdateDefaultRolesRequest struct {
	DefaultRoles map[string]*string `json:"default_roles" validate:"required"`
}

type UpdateAutoCreateEmployeeRequest struct {
	Enabled             bool    `json:"enabled"`
	DefaultDepartmentID *string `json:"default_department_id"`
}
