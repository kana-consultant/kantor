package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	backendauth "github.com/kana-consultant/kantor/backend/internal/auth"
	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	"github.com/kana-consultant/kantor/backend/internal/security"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

const (
	maxFailedLoginAttempts = 5
	accountLockDuration    = 15 * time.Minute
)

var (
	ErrEmailAlreadyExists     = errors.New("email sudah terdaftar")
	ErrInvalidCredentials     = errors.New("email atau kata sandi tidak valid")
	ErrInvalidCurrentPassword = errors.New("kata sandi saat ini tidak sesuai")
	ErrInactiveUser           = errors.New("akun pengguna sedang tidak aktif")
	ErrAccountLocked          = errors.New("akun dikunci sementara karena terlalu banyak percobaan login yang gagal")
	ErrInvalidRefreshToken    = errors.New("refresh token tidak valid")
	ErrExpiredRefreshToken    = errors.New("refresh token sudah kedaluwarsa")
	ErrPasswordUnchanged      = errors.New("kata sandi baru harus berbeda dari kata sandi saat ini")
	ErrEmailUnchanged         = errors.New("email baru harus berbeda dari email saat ini")
	ErrInvalidResetToken      = errors.New("tautan reset kata sandi tidak valid")
	ErrExpiredResetToken      = errors.New("tautan reset kata sandi sudah kedaluwarsa")
	ErrPasswordResetDisabled  = errors.New("fitur lupa kata sandi belum tersedia")
)

type authRepository interface {
	UpdateUserClientContext(ctx context.Context, userID string, params authrepo.UpdateUserClientContextParams) error
	EnsureUserWithRoles(ctx context.Context, params authrepo.CreateUserParams, roles []rbac.RoleKey) (model.User, error)
	CreateUserWithRoles(ctx context.Context, params authrepo.CreateUserParams, roles []rbac.RoleKey) (model.User, error)
	CreatePasswordResetToken(ctx context.Context, params authrepo.CreatePasswordResetTokenParams) error
	GetUserByEmail(ctx context.Context, email string) (model.User, error)
	GetUserByID(ctx context.Context, userID string) (model.User, error)
	GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (model.PasswordResetToken, error)
	GetDefaultRoleAssignments(ctx context.Context) ([]rbac.RoleKey, error)
	GetUserModuleRoles(ctx context.Context, userID string) (map[string]rbac.ModuleRole, error)
	GetEffectivePermissions(ctx context.Context, userID string) ([]string, error)
	GetUserRolesAndPermissions(ctx context.Context, userID string) ([]string, []string, error)
	CreateRefreshToken(ctx context.Context, params authrepo.CreateRefreshTokenParams) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (model.RefreshToken, error)
	RotateRefreshToken(ctx context.Context, oldTokenHash string, params authrepo.CreateRefreshTokenParams) error
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	IsUniqueViolation(err error) bool
	IncrementFailedLoginAttempts(ctx context.Context, userID string, maxAttempts int, lockDuration time.Duration) error
	ResetFailedLoginAttempts(ctx context.Context, userID string) error
	ChangePasswordAndRevokeTokens(ctx context.Context, userID string, passwordHash string) error
	ListUsers(ctx context.Context, params authrepo.ListUsersParams) ([]authrepo.UserWithRoles, int64, error)
	ReplaceUserRoles(ctx context.Context, userID string, roles []rbac.RoleKey) error
	SetUserActive(ctx context.Context, userID string, active bool) error
	UpdateUserFullNameAndPhone(ctx context.Context, userID string, fullName string, phone *string) error
	UpdateUserFields(ctx context.Context, userID string, fullName string, email string) error
	UpdateUserAvatar(ctx context.Context, userID string, avatarURL *string) error
	UpdateEmployeeEmailByUserID(ctx context.Context, userID string, email string) error
	UpdateEmployeeAvatarByUserID(ctx context.Context, userID string, avatarURL string) error
	ListRoles(ctx context.Context, params authrepo.RoleListParams) ([]authrepo.RoleListItem, error)
	GetRoleDetail(ctx context.Context, roleID string) (authrepo.RoleDetail, error)
	CreateRole(ctx context.Context, params authrepo.UpsertRoleParams, createdBy string) (authrepo.RoleDetail, error)
	UpdateRole(ctx context.Context, roleID string, params authrepo.UpsertRoleParams) (authrepo.RoleDetail, error)
	DeleteRole(ctx context.Context, roleID string) error
	ToggleRole(ctx context.Context, roleID string) (authrepo.RoleDetail, error)
	DuplicateRole(ctx context.Context, roleID string, createdBy string) (authrepo.RoleDetail, error)
	ListPermissionGroups(ctx context.Context) ([]authrepo.PermissionGroup, error)
	ListAdminUsers(ctx context.Context, params dto.ListUsersQuery) ([]authrepo.AdminUserSummary, int64, error)
	GetAdminUserDetail(ctx context.Context, userID string) (authrepo.AdminUserDetail, error)
	ReplaceUserModuleRoles(ctx context.Context, userID string, moduleRoles []dto.SetUserModuleRoleRequest) error
	SetUserSuperAdmin(ctx context.Context, userID string, enabled bool) error
	GetSettings(ctx context.Context) (authrepo.SettingsResponse, error)
	GetMailDeliveryRecord(ctx context.Context) (authrepo.MailDeliverySettingRecord, error)
	GetPublicAuthOptions(ctx context.Context) (authrepo.PublicAuthOptions, error)
	UpdateDefaultRoles(ctx context.Context, updatedBy string, mapping map[string]*string) error
	UpdateAutoCreateEmployee(ctx context.Context, updatedBy string, setting authrepo.AutoCreateEmployeeSetting) error
	UpdateMailDelivery(ctx context.Context, updatedBy string, setting authrepo.MailDeliverySettingRecord) error
	ListModules(ctx context.Context) ([]authrepo.ModuleItem, error)
	ListSettingsDepartments(ctx context.Context) ([]model.Department, error)
	EnsureEmployeeProfileForUser(ctx context.Context, userID string) (model.Employee, error)
	UsePasswordResetToken(ctx context.Context, tokenID string, userID string, passwordHash string) error
}

type authEmployeesRepository interface {
	GetEmployeeByUserID(ctx context.Context, userID string) (model.Employee, error)
	UpdateEmployeeProfile(
		ctx context.Context,
		userID string,
		fullName string,
		phone *string,
		address *string,
		emergencyContact *string,
		avatarURL *string,
		bankAccountNumber *string,
		bankName *string,
		linkedInProfile *string,
		sshKeys *string,
	) (model.Employee, error)
}

type Service struct {
	repo            authRepository
	employeeRepo    authEmployeesRepository
	tokenManager    *backendauth.TokenManager
	permissionCache *rbac.PermissionCache
	passwordMailer  passwordResetMailer
	encrypter       *security.Encrypter
	fallbackAppURL  string
}

type AuthResult struct {
	User         model.User
	ModuleRoles  map[string]dto.ModuleRoleDTO
	Permissions  []string
	IsSuperAdmin bool
	Tokens       dto.TokenPair
}

type PasswordResetRequestMeta struct {
	PublicBaseURL string
	UserAgent     string
	IPAddress     string
}

type mailDeliveryRuntimeConfig struct {
	SenderName           string
	SenderEmail          string
	ReplyToEmail         *string
	APIKey               string
	PasswordResetTTL     time.Duration
}

func New(repo authRepository, employeeRepo authEmployeesRepository, cfg config.Config, permissionCache *rbac.PermissionCache, encrypter *security.Encrypter) *Service {
	return &Service{
		repo:            repo,
		employeeRepo:    employeeRepo,
		tokenManager:    backendauth.NewTokenManager(cfg.JWTSecret, cfg.JWTAccessExpiry, cfg.JWTRefreshExpiry),
		permissionCache: permissionCache,
		passwordMailer:  newResendMailer(),
		encrypter:       encrypter,
		fallbackAppURL:  strings.TrimRight(strings.TrimSpace(cfg.AppURL), "/"),
	}
}

func (s *Service) EnsureSeedSuperAdmin(ctx context.Context, email string, password string, fullName string) error {
	return s.EnsureSeedUserWithRoles(ctx, authrepo.CreateUserParams{
		Email:        strings.ToLower(strings.TrimSpace(email)),
		PasswordHash: "",
		FullName:     strings.TrimSpace(fullName),
	}, []rbac.RoleKey{{Name: "super_admin"}}, password)
}

func (s *Service) EnsureSeedUserWithRoles(ctx context.Context, params authrepo.CreateUserParams, roles []rbac.RoleKey, rawPassword string) error {
	passwordHash, err := backendauth.HashPassword(rawPassword)
	if err != nil {
		return err
	}

	params.Email = strings.ToLower(strings.TrimSpace(params.Email))
	params.PasswordHash = passwordHash
	params.FullName = strings.TrimSpace(params.FullName)

	if params.Department != nil {
		trimmed := strings.TrimSpace(*params.Department)
		params.Department = &trimmed
	}

	_, err = s.repo.EnsureUserWithRoles(ctx, params, roles)
	return err
}

func (s *Service) Register(ctx context.Context, input dto.RegisterRequest, userAgent string, ipAddress string) (AuthResult, error) {
	defaultRoles, err := s.repo.GetDefaultRoleAssignments(ctx)
	if err != nil {
		return AuthResult{}, err
	}

	passwordHash, err := backendauth.HashPassword(input.Password)
	if err != nil {
		return AuthResult{}, err
	}

	user, err := s.repo.CreateUserWithRoles(ctx, authrepo.CreateUserParams{
		Email:        strings.ToLower(strings.TrimSpace(input.Email)),
		PasswordHash: passwordHash,
		FullName:     strings.TrimSpace(input.FullName),
	}, defaultRoles)
	if err != nil {
		if s.repo.IsUniqueViolation(err) {
			return AuthResult{}, ErrEmailAlreadyExists
		}

		return AuthResult{}, err
	}

	return s.issueAuthResult(ctx, user, "", userAgent, ipAddress)
}

func (s *Service) Login(ctx context.Context, input dto.LoginRequest, userAgent string, ipAddress string) (AuthResult, error) {
	user, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(input.Email)))
	if err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}

		return AuthResult{}, err
	}

	if !user.IsActive {
		return AuthResult{}, ErrInactiveUser
	}

	// Always run bcrypt before checking lock status to prevent
	// timing oracle that reveals whether an account is locked.
	passwordErr := backendauth.ComparePassword(user.PasswordHash, input.Password)

	if user.LockedUntil != nil && user.LockedUntil.After(time.Now().UTC()) {
		return AuthResult{}, ErrAccountLocked
	}

	if passwordErr != nil {
		if err := s.repo.IncrementFailedLoginAttempts(ctx, user.ID, maxFailedLoginAttempts, accountLockDuration); err != nil {
			slog.Error("failed to increment login attempts", "error", err, "user_id", user.ID)
		}
		return AuthResult{}, ErrInvalidCredentials
	}

	if user.FailedLoginAttempts > 0 {
		_ = s.repo.ResetFailedLoginAttempts(ctx, user.ID)
	}

	return s.issueAuthResult(ctx, user, "", userAgent, ipAddress)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string, userAgent string, ipAddress string) (AuthResult, error) {
	tokenHash := backendauth.HashRefreshToken(refreshToken)

	storedToken, err := s.repo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return AuthResult{}, ErrInvalidRefreshToken
		}

		return AuthResult{}, err
	}

	if storedToken.RevokedAt != nil {
		return AuthResult{}, ErrInvalidRefreshToken
	}

	if storedToken.ExpiresAt.Before(time.Now().UTC()) {
		return AuthResult{}, ErrExpiredRefreshToken
	}

	user, err := s.repo.GetUserByID(ctx, storedToken.UserID)
	if err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return AuthResult{}, ErrInvalidRefreshToken
		}

		return AuthResult{}, err
	}

	if !user.IsActive {
		return AuthResult{}, ErrInactiveUser
	}

	return s.issueAuthResult(ctx, user, tokenHash, userAgent, ipAddress)
}

func (s *Service) ChangePassword(ctx context.Context, userID string, currentPassword string, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := backendauth.ComparePassword(user.PasswordHash, currentPassword); err != nil {
		return ErrInvalidCurrentPassword
	}

	if currentPassword == newPassword {
		return ErrPasswordUnchanged
	}

	newHash, err := backendauth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	return s.repo.ChangePasswordAndRevokeTokens(ctx, userID, newHash)
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string, meta PasswordResetRequestMeta) error {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return nil
	}

	mailConfig, err := s.getMailDeliveryRuntimeConfig(ctx)
	if err != nil {
		return err
	}

	user, err := s.repo.GetUserByEmail(ctx, normalizedEmail)
	if err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return nil
		}
		return err
	}

	if !user.IsActive {
		return nil
	}

	baseURL := strings.TrimRight(strings.TrimSpace(meta.PublicBaseURL), "/")
	if baseURL == "" {
		baseURL = s.fallbackAppURL
	}
	if baseURL == "" {
		return ErrPasswordResetDisabled
	}

	rawToken, tokenHash, err := generatePasswordResetToken()
	if err != nil {
		return err
	}

	expiresAt := time.Now().UTC().Add(mailConfig.PasswordResetTTL)
	if err := s.repo.CreatePasswordResetToken(ctx, authrepo.CreatePasswordResetTokenParams{
		UserID:      user.ID,
		TokenHash:   tokenHash,
		ExpiresAt:   expiresAt,
		RequestedIP: meta.IPAddress,
		UserAgent:   meta.UserAgent,
	}); err != nil {
		return err
	}

	tenantName := "Kantor"
	if info, ok := tenant.FromContext(ctx); ok && strings.TrimSpace(info.Name) != "" {
		tenantName = info.Name
	}

	resetURL := baseURL + "/reset-password?token=" + url.QueryEscape(rawToken)
	if err := s.passwordMailer.SendPasswordReset(ctx, resendMailConfig{
		APIKey:       mailConfig.APIKey,
		SenderName:   mailConfig.SenderName,
		SenderEmail:  mailConfig.SenderEmail,
		ReplyToEmail: mailConfig.ReplyToEmail,
	}, passwordResetEmail{
		ToEmail:    user.Email,
		ToName:     user.FullName,
		TenantName: tenantName,
		ResetURL:   resetURL,
		ExpiresIn:  mailConfig.PasswordResetTTL,
	}); err != nil {
		return err
	}

	return nil
}

func (s *Service) ValidatePasswordResetToken(ctx context.Context, rawToken string) error {
	token, err := s.repo.GetPasswordResetTokenByHash(ctx, hashPasswordResetToken(rawToken))
	if err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return ErrInvalidResetToken
		}
		return err
	}

	if token.UsedAt != nil {
		return ErrInvalidResetToken
	}

	if token.ExpiresAt.Before(time.Now().UTC()) {
		return ErrExpiredResetToken
	}

	return nil
}

func (s *Service) ResetPasswordWithToken(ctx context.Context, rawToken string, newPassword string) (string, error) {
	token, err := s.repo.GetPasswordResetTokenByHash(ctx, hashPasswordResetToken(rawToken))
	if err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return "", ErrInvalidResetToken
		}
		return "", err
	}

	if token.UsedAt != nil {
		return "", ErrInvalidResetToken
	}

	if token.ExpiresAt.Before(time.Now().UTC()) {
		return "", ErrExpiredResetToken
	}

	user, err := s.repo.GetUserByID(ctx, token.UserID)
	if err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return "", ErrInvalidResetToken
		}
		return "", err
	}

	if backendauth.ComparePassword(user.PasswordHash, newPassword) == nil {
		return "", ErrPasswordUnchanged
	}

	newHash, err := backendauth.HashPassword(newPassword)
	if err != nil {
		return "", err
	}

	if err := s.repo.UsePasswordResetToken(ctx, token.ID, token.UserID, newHash); err != nil {
		return "", err
	}

	return token.UserID, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) (string, error) {
	tokenHash := backendauth.HashRefreshToken(strings.TrimSpace(refreshToken))
	if tokenHash == "" {
		return "", ErrInvalidRefreshToken
	}

	storedToken, err := s.repo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return "", ErrInvalidRefreshToken
		}
		return "", err
	}

	if err := s.repo.RevokeRefreshToken(ctx, tokenHash); err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return "", ErrInvalidRefreshToken
		}
		return "", err
	}

	return storedToken.UserID, nil
}

func (s *Service) ParseAccessToken(token string) (*backendauth.AccessClaims, error) {
	return s.tokenManager.ParseAccessToken(token)
}

func (s *Service) GetSession(ctx context.Context, userID string) (AuthResult, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return AuthResult{}, err
	}

	cachedPermissions, err := s.permissionCache.Load(ctx, userID)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		User:         user,
		ModuleRoles:  toModuleRoleDTOs(cachedPermissions.ModuleRoles),
		Permissions:  cachedPermissions.PermissionList(),
		IsSuperAdmin: cachedPermissions.IsSuperAdmin,
	}, nil
}

func (s *Service) UpdateClientContext(ctx context.Context, userID string, input dto.UpdateClientContextRequest) (model.User, error) {
	if err := s.repo.UpdateUserClientContext(ctx, userID, authrepo.UpdateUserClientContextParams{
		Timezone:              input.Timezone,
		TimezoneOffsetMinutes: input.TimezoneOffsetMinutes,
		Locale:                input.Locale,
	}); err != nil {
		return model.User{}, err
	}

	return s.repo.GetUserByID(ctx, userID)
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

func (s *Service) GetProfile(ctx context.Context, userID string) (model.Employee, error) {
	return s.employeeRepo.GetEmployeeByUserID(ctx, userID)
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, input dto.UpdateProfileRequest) (model.Employee, error) {
	fullName := strings.TrimSpace(input.FullName)

	employee, err := s.employeeRepo.UpdateEmployeeProfile(
		ctx,
		userID,
		fullName,
		input.Phone,
		input.Address,
		input.EmergencyContact,
		input.AvatarURL,
		input.BankAccountNumber,
		input.BankName,
		input.LinkedInProfile,
		input.SSHKeys,
	)
	if err != nil {
		return model.Employee{}, err
	}

	// Sync full_name + phone to users table (phone is used by WA broadcast)
	if err := s.repo.UpdateUserFullNameAndPhone(ctx, userID, fullName, input.Phone); err != nil {
		return model.Employee{}, err
	}
	if err := s.repo.UpdateUserAvatar(ctx, userID, employee.AvatarURL); err != nil {
		return model.Employee{}, err
	}

	return employee, nil
}

func (s *Service) ChangeEmail(ctx context.Context, userID string, newEmail string, password string) error {
	newEmail = strings.ToLower(strings.TrimSpace(newEmail))

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if backendauth.ComparePassword(user.PasswordHash, password) != nil {
		return ErrInvalidCurrentPassword
	}

	if strings.EqualFold(user.Email, newEmail) {
		return ErrEmailUnchanged
	}

	if err := s.repo.UpdateUserFields(ctx, userID, user.FullName, newEmail); err != nil {
		if s.repo.IsUniqueViolation(err) {
			return ErrEmailAlreadyExists
		}
		return err
	}

	// Also sync to employees table
	if err := s.repo.UpdateEmployeeEmailByUserID(ctx, userID, newEmail); err != nil {
		slog.Warn("failed to sync email to employee", "user_id", userID, "error", err)
	}

	return nil
}

func (s *Service) UpdateProfileAvatar(ctx context.Context, userID string, avatarURL string) error {
	// Update employee avatar
	if err := s.repo.UpdateEmployeeAvatarByUserID(ctx, userID, avatarURL); err != nil {
		slog.Warn("failed to sync avatar to employee", "user_id", userID, "error", err)
	}
	// Sync to users table
	return s.repo.UpdateUserAvatar(ctx, userID, &avatarURL)
}

// ---------------------------------------------------------------------------
// User management (admin)
// ---------------------------------------------------------------------------

func (s *Service) ListUsers(ctx context.Context, params authrepo.ListUsersParams) ([]authrepo.UserWithRoles, int64, error) {
	return s.repo.ListUsers(ctx, params)
}

func (s *Service) GetUserWithRoles(ctx context.Context, userID string) (*authrepo.UserWithRoles, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles, permissions, err := s.repo.GetUserRolesAndPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &authrepo.UserWithRoles{User: user, Roles: roles, Permissions: permissions}, nil
}

func (s *Service) UpdateUserRoles(ctx context.Context, userID string, roles []rbac.RoleKey) error {
	// Verify user exists
	if _, err := s.repo.GetUserByID(ctx, userID); err != nil {
		return err
	}
	if err := s.repo.ReplaceUserRoles(ctx, userID, roles); err != nil {
		return err
	}
	s.permissionCache.Invalidate(ctx, userID)
	return nil
}

func (s *Service) SetUserActive(ctx context.Context, userID string, active bool) error {
	return s.repo.SetUserActive(ctx, userID, active)
}

func (s *Service) ListRoles(ctx context.Context, params authrepo.RoleListParams) ([]authrepo.RoleListItem, error) {
	return s.repo.ListRoles(ctx, params)
}

func (s *Service) GetRoleDetail(ctx context.Context, roleID string) (authrepo.RoleDetail, error) {
	return s.repo.GetRoleDetail(ctx, roleID)
}

func (s *Service) CreateRole(ctx context.Context, params authrepo.UpsertRoleParams, createdBy string) (authrepo.RoleDetail, error) {
	return s.repo.CreateRole(ctx, params, createdBy)
}

func (s *Service) UpdateRole(ctx context.Context, roleID string, params authrepo.UpsertRoleParams) (authrepo.RoleDetail, error) {
	current, err := s.repo.GetRoleDetail(ctx, roleID)
	if err != nil {
		return authrepo.RoleDetail{}, err
	}

	updated, err := s.repo.UpdateRole(ctx, roleID, params)
	if err != nil {
		return authrepo.RoleDetail{}, err
	}

	s.permissionCache.InvalidateByRole(current.ID)
	return updated, nil
}

func (s *Service) DeleteRole(ctx context.Context, roleID string) error {
	current, err := s.repo.GetRoleDetail(ctx, roleID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteRole(ctx, roleID); err != nil {
		return err
	}
	s.permissionCache.InvalidateByRole(current.ID)
	return nil
}

func (s *Service) ToggleRole(ctx context.Context, roleID string) (authrepo.RoleDetail, error) {
	updated, err := s.repo.ToggleRole(ctx, roleID)
	if err != nil {
		return authrepo.RoleDetail{}, err
	}
	s.permissionCache.InvalidateByRole(roleID)
	return updated, nil
}

func (s *Service) DuplicateRole(ctx context.Context, roleID string, createdBy string) (authrepo.RoleDetail, error) {
	return s.repo.DuplicateRole(ctx, roleID, createdBy)
}

func (s *Service) ListPermissionGroups(ctx context.Context) ([]authrepo.PermissionGroup, error) {
	return s.repo.ListPermissionGroups(ctx)
}

func (s *Service) ListAdminUsers(ctx context.Context, params dto.ListUsersQuery) ([]authrepo.AdminUserSummary, int64, error) {
	return s.repo.ListAdminUsers(ctx, params)
}

func (s *Service) GetAdminUserDetail(ctx context.Context, userID string) (authrepo.AdminUserDetail, error) {
	return s.repo.GetAdminUserDetail(ctx, userID)
}

func (s *Service) EnsureEmployeeProfileForUser(ctx context.Context, userID string) (authrepo.AdminUserDetail, error) {
	if _, err := s.repo.EnsureEmployeeProfileForUser(ctx, userID); err != nil {
		return authrepo.AdminUserDetail{}, err
	}
	return s.repo.GetAdminUserDetail(ctx, userID)
}

func (s *Service) UpdateUserModuleRoles(ctx context.Context, userID string, moduleRoles []dto.SetUserModuleRoleRequest) error {
	if _, err := s.repo.GetUserByID(ctx, userID); err != nil {
		return err
	}
	if err := s.repo.ReplaceUserModuleRoles(ctx, userID, moduleRoles); err != nil {
		return err
	}
	s.permissionCache.Invalidate(ctx, userID)
	return nil
}

func (s *Service) ToggleUserSuperAdmin(ctx context.Context, actorID string, targetUserID string, enabled bool) error {
	if actorID == targetUserID {
		return authrepo.ErrCannotToggleSelf
	}
	if err := s.repo.SetUserSuperAdmin(ctx, targetUserID, enabled); err != nil {
		return err
	}
	s.permissionCache.Invalidate(ctx, targetUserID)
	return nil
}

func (s *Service) GetSettings(ctx context.Context) (authrepo.SettingsResponse, error) {
	return s.repo.GetSettings(ctx)
}

func (s *Service) GetPublicAuthOptions(ctx context.Context) (authrepo.PublicAuthOptions, error) {
	return s.repo.GetPublicAuthOptions(ctx)
}

func (s *Service) UpdateDefaultRoles(ctx context.Context, updatedBy string, mapping map[string]*string) error {
	return s.repo.UpdateDefaultRoles(ctx, updatedBy, mapping)
}

func (s *Service) UpdateAutoCreateEmployee(ctx context.Context, updatedBy string, setting authrepo.AutoCreateEmployeeSetting) error {
	return s.repo.UpdateAutoCreateEmployee(ctx, updatedBy, setting)
}

func (s *Service) UpdateMailDelivery(ctx context.Context, updatedBy string, input dto.UpdateMailDeliveryRequest) error {
	existing, err := s.repo.GetMailDeliveryRecord(ctx)
	if err != nil {
		return err
	}

	updated := existing
	updated.Enabled = input.Enabled
	updated.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	if updated.Provider == "" {
		updated.Provider = "resend"
	}
	updated.SenderName = strings.TrimSpace(input.SenderName)
	updated.SenderEmail = strings.ToLower(strings.TrimSpace(input.SenderEmail))
	updated.PasswordResetEnabled = input.PasswordResetEnabled
	updated.PasswordResetExpiryMinutes = input.PasswordResetExpiryMinutes
	updated.NotificationEnabled = input.NotificationEnabled

	if input.ReplyToEmail != nil {
		trimmed := strings.ToLower(strings.TrimSpace(*input.ReplyToEmail))
		if trimmed == "" {
			updated.ReplyToEmail = nil
		} else {
			updated.ReplyToEmail = &trimmed
		}
	}

	if input.ClearAPIKey {
		updated.APIKeyEncrypted = ""
	} else if input.APIKey != nil {
		trimmedKey := strings.TrimSpace(*input.APIKey)
		if trimmedKey != "" {
			if s.encrypter == nil {
				return errors.New("mail encrypter is not configured")
			}
			ciphertext, err := s.encrypter.EncryptString(trimmedKey)
			if err != nil {
				return err
			}
			updated.APIKeyEncrypted = ciphertext
		}
	}

	return s.repo.UpdateMailDelivery(ctx, updatedBy, updated)
}

func (s *Service) ListModules(ctx context.Context) ([]authrepo.ModuleItem, error) {
	return s.repo.ListModules(ctx)
}

func (s *Service) ListSettingsDepartments(ctx context.Context) ([]model.Department, error) {
	return s.repo.ListSettingsDepartments(ctx)
}

func (s *Service) getMailDeliveryRuntimeConfig(ctx context.Context) (mailDeliveryRuntimeConfig, error) {
	setting, err := s.repo.GetMailDeliveryRecord(ctx)
	if err != nil {
		return mailDeliveryRuntimeConfig{}, err
	}
	if !setting.ForgotPasswordEnabled() {
		return mailDeliveryRuntimeConfig{}, ErrPasswordResetDisabled
	}
	if s.encrypter == nil {
		return mailDeliveryRuntimeConfig{}, ErrPasswordResetDisabled
	}

	apiKey, err := s.encrypter.DecryptString(setting.APIKeyEncrypted)
	if err != nil {
		return mailDeliveryRuntimeConfig{}, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return mailDeliveryRuntimeConfig{}, ErrPasswordResetDisabled
	}

	return mailDeliveryRuntimeConfig{
		SenderName:       setting.SenderName,
		SenderEmail:      setting.SenderEmail,
		ReplyToEmail:     setting.ReplyToEmail,
		APIKey:           apiKey,
		PasswordResetTTL: time.Duration(setting.PasswordResetExpiryMinutes) * time.Minute,
	}, nil
}

func (s *Service) issueAuthResult(ctx context.Context, user model.User, oldTokenHash string, userAgent string, ipAddress string) (AuthResult, error) {
	cachedPermissions, err := s.permissionCache.Load(ctx, user.ID)
	if err != nil {
		return AuthResult{}, err
	}

	now := time.Now().UTC()
	var tenantID string
	if info, ok := tenant.FromContext(ctx); ok {
		tenantID = info.ID
	}
	accessToken, expiresAt, err := s.tokenManager.GenerateAccessToken(user.ID, tenantID, now)
	if err != nil {
		return AuthResult{}, err
	}

	refreshToken, refreshExpiresAt, err := s.tokenManager.GenerateRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}

	refreshParams := authrepo.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: backendauth.HashRefreshToken(refreshToken),
		ExpiresAt: refreshExpiresAt,
		UserAgent: userAgent,
		IPAddress: ipAddress,
	}

	if oldTokenHash == "" {
		if err := s.repo.CreateRefreshToken(ctx, refreshParams); err != nil {
			return AuthResult{}, err
		}
	} else {
		if err := s.repo.RotateRefreshToken(ctx, oldTokenHash, refreshParams); err != nil {
			return AuthResult{}, err
		}
	}

	return AuthResult{
		User:         user,
		ModuleRoles:  toModuleRoleDTOs(cachedPermissions.ModuleRoles),
		Permissions:  cachedPermissions.PermissionList(),
		IsSuperAdmin: cachedPermissions.IsSuperAdmin,
		Tokens: dto.TokenPair{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
			ExpiresIn:    int64(time.Until(expiresAt).Seconds()),
		},
	}, nil
}

func toModuleRoleDTOs(items map[string]rbac.ModuleRole) map[string]dto.ModuleRoleDTO {
	moduleRoles := make(map[string]dto.ModuleRoleDTO, len(items))
	for moduleID, role := range items {
		moduleRoles[moduleID] = dto.ModuleRoleDTO{
			RoleID:   role.RoleID,
			RoleName: role.RoleName,
			RoleSlug: role.RoleSlug,
		}
	}
	return moduleRoles
}

func generatePasswordResetToken() (string, string, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return "", "", err
	}

	rawToken := base64.RawURLEncoding.EncodeToString(entropy)
	return rawToken, hashPasswordResetToken(rawToken), nil
}

func hashPasswordResetToken(rawToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawToken)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
