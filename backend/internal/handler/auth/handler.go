package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
)

const refreshTokenCookie = "refresh_token"

type Handler struct {
	service       *authservice.Service
	validator     *validator.Validate
	cookieSecure  bool
	cookiePath    string
	refreshExpiry time.Duration
}

func New(service *authservice.Service, cfg config.Config) *Handler {
	return &Handler{
		service:       service,
		validator:     validator.New(validator.WithRequiredStructEnabled()),
		cookieSecure:  cfg.AppEnv == "production",
		cookiePath:    "/api/v1/auth",
		refreshExpiry: cfg.JWTRefreshExpiry,
	}
}

func (h *Handler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.NewIPRateLimit(5, time.Minute,
		"AUTH_RATE_LIMITED", "Too many registration attempts. Try again later.",
	)).Post("/register", h.register)
	router.With(platformmiddleware.NewIPRateLimit(10, time.Minute,
		"AUTH_RATE_LIMITED", "Too many login attempts. Try again later.",
	)).Post("/login", h.login)
	router.With(platformmiddleware.NewIPRateLimit(30, time.Minute,
		"AUTH_RATE_LIMITED", "Too many token refresh attempts. Try again later.",
	)).Post("/refresh", h.refresh)
	router.Post("/logout", h.logout)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.GetSession(r.Context(), principal.UserID)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"user":           result.User,
		"module_roles":   result.ModuleRoles,
		"permissions":    result.Permissions,
		"is_super_admin": result.IsSuperAdmin,
	}, nil)
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var input dto.ChangePasswordRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	if err := h.service.ChangePassword(r.Context(), principal.UserID, input.CurrentPassword, input.NewPassword); err != nil {
		h.writeAuthError(w, err)
		return
	}

	platformmiddleware.AuditLogWithUser(r.Context(), principal.UserID, "update", "admin", "password", principal.UserID, nil, map[string]any{
		"changed": true,
	})

	response.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Password changed successfully. All sessions have been revoked.",
	}, nil)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var input dto.RegisterRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.Register(r.Context(), input, r.UserAgent(), clientIP(r))
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	h.setRefreshTokenCookie(w, result.Tokens.RefreshToken)
	platformmiddleware.AuditLogWithUser(r.Context(), result.User.ID, "register", "admin", "auth", result.User.ID, nil, map[string]any{
		"email":          result.User.Email,
		"is_super_admin": result.IsSuperAdmin,
		"module_roles":   result.ModuleRoles,
	})
	response.WriteJSON(w, http.StatusCreated, dto.AuthResponse{
		User:         result.User,
		ModuleRoles:  result.ModuleRoles,
		Permissions:  result.Permissions,
		IsSuperAdmin: result.IsSuperAdmin,
		Tokens:       result.Tokens,
	}, nil)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input dto.LoginRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.Login(r.Context(), input, r.UserAgent(), clientIP(r))
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	h.setRefreshTokenCookie(w, result.Tokens.RefreshToken)
	platformmiddleware.AuditLogWithUser(r.Context(), result.User.ID, "login", "admin", "auth", result.User.ID, nil, map[string]any{
		"email":          result.User.Email,
		"is_super_admin": result.IsSuperAdmin,
	})
	response.WriteJSON(w, http.StatusOK, dto.AuthResponse{
		User:         result.User,
		ModuleRoles:  result.ModuleRoles,
		Permissions:  result.Permissions,
		IsSuperAdmin: result.IsSuperAdmin,
		Tokens:       result.Tokens,
	}, nil)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := h.readRefreshTokenCookie(r)
	if err != nil {
		response.WriteError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Refresh token cookie is missing", nil)
		return
	}

	result, err := h.service.Refresh(r.Context(), refreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	h.setRefreshTokenCookie(w, result.Tokens.RefreshToken)
	response.WriteJSON(w, http.StatusOK, dto.AuthResponse{
		User:         result.User,
		ModuleRoles:  result.ModuleRoles,
		Permissions:  result.Permissions,
		IsSuperAdmin: result.IsSuperAdmin,
		Tokens:       result.Tokens,
	}, nil)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := h.readRefreshTokenCookie(r)
	if err == nil {
		if userID, logoutErr := h.service.Logout(r.Context(), refreshToken); logoutErr == nil {
			platformmiddleware.AuditLogWithUser(r.Context(), userID, "logout", "admin", "auth", userID, nil, map[string]any{
				"revoked": true,
			})
		}
	}

	h.clearRefreshTokenCookie(w)
	response.WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true}, nil)
}

func (h *Handler) setRefreshTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    token,
		Path:     h.cookiePath,
		MaxAge:   int(h.refreshExpiry.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearRefreshTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    "",
		Path:     h.cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) readRefreshTokenCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie(refreshTokenCookie)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func (h *Handler) decodeAndValidate(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return false
	}

	if err := h.validator.Struct(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return false
	}

	return true
}

func (h *Handler) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authservice.ErrEmailAlreadyExists):
		response.WriteError(w, http.StatusConflict, "EMAIL_ALREADY_EXISTS", err.Error(), nil)
	case errors.Is(err, authservice.ErrInvalidCredentials):
		response.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", err.Error(), nil)
	case errors.Is(err, authservice.ErrInactiveUser):
		response.WriteError(w, http.StatusForbidden, "INACTIVE_USER", err.Error(), nil)
	case errors.Is(err, authservice.ErrAccountLocked):
		response.WriteError(w, http.StatusTooManyRequests, "ACCOUNT_LOCKED", err.Error(), nil)
	case errors.Is(err, authservice.ErrInvalidRefreshToken), errors.Is(err, authservice.ErrExpiredRefreshToken):
		response.WriteError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", err.Error(), nil)
	case errors.Is(err, authservice.ErrPasswordUnchanged):
		response.WriteError(w, http.StatusBadRequest, "PASSWORD_UNCHANGED", err.Error(), nil)
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}

func validationDetails(err error) map[string]string {
	details := map[string]string{}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return details
	}

	for _, validationErr := range validationErrors {
		details[validationErr.Field()] = validationErr.Tag()
	}

	return details
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}
