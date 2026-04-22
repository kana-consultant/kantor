package dto

type RegisterRequest struct {
	Email            string `json:"email" validate:"required,email"`
	Password         string `json:"password" validate:"required,min=8"`
	FullName         string `json:"full_name" validate:"required,min=3,max=120"`
	RegistrationCode string `json:"registration_code" validate:"required,min=8,max=128"`
}

type UpdateRegistrationSettingsRequest struct {
	Enabled              bool     `json:"enabled"`
	RotationIntervalDays int      `json:"rotation_interval_days" validate:"omitempty,min=1,max=90"`
	AllowedEmailDomains  []string `json:"allowed_email_domains"`
}

type RollRegistrationCodeResponse struct {
	Code     string      `json:"code"`
	Settings interface{} `json:"settings"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required,min=24"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
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
