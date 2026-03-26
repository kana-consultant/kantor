package dto

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	FullName string `json:"full_name" validate:"required,min=3,max=120"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"-"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type ExtensionTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type ModuleRoleDTO struct {
	RoleID   string `json:"role_id"`
	RoleName string `json:"role_name"`
	RoleSlug string `json:"role_slug"`
}

type AuthResponse struct {
	User         interface{}               `json:"user"`
	ModuleRoles  map[string]ModuleRoleDTO `json:"module_roles"`
	Permissions  []string                 `json:"permissions"`
	IsSuperAdmin bool                     `json:"is_super_admin"`
	Tokens       TokenPair                `json:"tokens"`
}
