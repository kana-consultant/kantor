package dto

type ListUsersQuery struct {
	Page    int    `validate:"omitempty,min=1"`
	PerPage int    `validate:"omitempty,min=1,max=100"`
	Search  string `validate:"omitempty,max=150"`
}

type RoleKeyDTO struct {
	Name   string `json:"name" validate:"required"`
	Module string `json:"module" validate:"omitempty"`
}

type UpdateUserRolesRequest struct {
	Roles []RoleKeyDTO `json:"roles" validate:"required,min=1,dive"`
}

type ToggleActiveRequest struct {
	Active bool `json:"active"`
}
