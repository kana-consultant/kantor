package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	backendauth "github.com/kana-consultant/kantor/backend/internal/auth"
	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

var (
	ErrEmailAlreadyExists  = errors.New("email already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInactiveUser        = errors.New("user account is inactive")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrExpiredRefreshToken = errors.New("refresh token has expired")
)

type Service struct {
	repo         *authrepo.Repository
	employeeRepo *hrisrepo.EmployeesRepository
	tokenManager *backendauth.TokenManager
}

type AuthResult struct {
	User        model.User
	Roles       []string
	Permissions []string
	Tokens      dto.TokenPair
}

func New(repo *authrepo.Repository, employeeRepo *hrisrepo.EmployeesRepository, cfg config.Config) *Service {
	return &Service{
		repo:         repo,
		employeeRepo: employeeRepo,
		tokenManager: backendauth.NewTokenManager(cfg.JWTSecret, cfg.JWTAccessExpiry, cfg.JWTRefreshExpiry),
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
	existingUsers, err := s.repo.CountUsers(ctx)
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
	}, rbac.DefaultRolesForNewUser(existingUsers))
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

	if err := backendauth.ComparePassword(user.PasswordHash, input.Password); err != nil {
		return AuthResult{}, ErrInvalidCredentials
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

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	tokenHash := backendauth.HashRefreshToken(strings.TrimSpace(refreshToken))
	if tokenHash == "" {
		return ErrInvalidRefreshToken
	}

	if err := s.repo.RevokeRefreshToken(ctx, tokenHash); err != nil {
		if errors.Is(err, authrepo.ErrNotFound) {
			return ErrInvalidRefreshToken
		}
		return err
	}

	return nil
}

func (s *Service) ParseAccessToken(token string) (*backendauth.AccessClaims, error) {
	return s.tokenManager.ParseAccessToken(token)
}

func (s *Service) GetSession(ctx context.Context, userID string) (AuthResult, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return AuthResult{}, err
	}

	roles, permissions, err := s.repo.GetUserRolesAndPermissions(ctx, userID)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		User:        user,
		Roles:       roles,
		Permissions: permissions,
	}, nil
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

func (s *Service) GetProfile(ctx context.Context, userID string) (model.Employee, error) {
	return s.employeeRepo.GetEmployeeByUserID(ctx, userID)
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, input dto.UpdateProfileRequest) (model.Employee, error) {
	fullName := strings.TrimSpace(input.FullName)

	employee, err := s.employeeRepo.UpdateEmployeeProfile(ctx, userID, fullName, input.Phone, input.Address, input.EmergencyContact, input.AvatarURL)
	if err != nil {
		return model.Employee{}, err
	}

	// Sync full_name + phone to users table (phone is used by WA broadcast)
	if err := s.repo.UpdateUserFullNameAndPhone(ctx, userID, fullName, input.Phone); err != nil {
		return model.Employee{}, err
	}

	return employee, nil
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
	roles, _, err := s.repo.GetUserRolesAndPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &authrepo.UserWithRoles{User: user, Roles: roles}, nil
}

func (s *Service) UpdateUserRoles(ctx context.Context, userID string, roles []rbac.RoleKey) error {
	// Verify user exists
	if _, err := s.repo.GetUserByID(ctx, userID); err != nil {
		return err
	}
	return s.repo.ReplaceUserRoles(ctx, userID, roles)
}

func (s *Service) SetUserActive(ctx context.Context, userID string, active bool) error {
	return s.repo.SetUserActive(ctx, userID, active)
}

func (s *Service) issueAuthResult(ctx context.Context, user model.User, oldTokenHash string, userAgent string, ipAddress string) (AuthResult, error) {
	roles, permissions, err := s.repo.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return AuthResult{}, err
	}

	now := time.Now().UTC()
	accessToken, expiresAt, err := s.tokenManager.GenerateAccessToken(user.ID, roles, permissions, now)
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
		User:        user,
		Roles:       roles,
		Permissions: permissions,
		Tokens: dto.TokenPair{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
			ExpiresIn:    int64(time.Until(expiresAt).Seconds()),
		},
	}, nil
}
