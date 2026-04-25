package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
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
	uploadsDir    string
}

func New(service *authservice.Service, cfg config.Config) *Handler {
	return &Handler{
		service:       service,
		validator:     validator.New(validator.WithRequiredStructEnabled()),
		cookieSecure:  cfg.AppEnv == "production",
		cookiePath:    "/api/v1/auth",
		refreshExpiry: cfg.JWTRefreshExpiry,
		uploadsDir:    cfg.UploadsDir,
	}
}

func (h *Handler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.NewIPRateLimit(5, time.Minute,
		"AUTH_RATE_LIMITED", "Terlalu banyak percobaan pendaftaran. Coba lagi nanti.",
	)).Post("/register", h.register)
	router.With(platformmiddleware.NewIPRateLimit(10, time.Minute,
		"AUTH_RATE_LIMITED", "Terlalu banyak percobaan login. Coba lagi nanti.",
	)).Post("/login", h.login)
	router.Get("/public-options", h.publicOptions)
	router.With(platformmiddleware.NewIPRateLimit(5, time.Minute,
		"AUTH_RATE_LIMITED", "Terlalu banyak permintaan reset kata sandi. Coba lagi nanti.",
	)).Post("/forgot-password", h.forgotPassword)
	router.With(platformmiddleware.NewIPRateLimit(20, time.Minute,
		"AUTH_RATE_LIMITED", "Terlalu banyak validasi link reset. Coba lagi nanti.",
	)).Get("/reset-password/validate", h.validateResetPassword)
	router.With(platformmiddleware.NewIPRateLimit(10, time.Minute,
		"AUTH_RATE_LIMITED", "Terlalu banyak percobaan reset kata sandi. Coba lagi nanti.",
	)).Post("/reset-password", h.resetPassword)
	router.With(platformmiddleware.NewIPRateLimit(30, time.Minute,
		"AUTH_RATE_LIMITED", "Terlalu banyak percobaan refresh token. Coba lagi nanti.",
	)).Post("/refresh", h.refresh)
	router.Post("/logout", h.logout)
}

func (h *Handler) UpdateClientContext(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi login tidak ditemukan", nil)
		return
	}

	var input dto.UpdateClientContextRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	user, err := h.service.UpdateClientContext(r.Context(), principal.UserID, input)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Gagal menyimpan konteks browser")
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]any{
		"user": user,
	}, nil)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi login tidak ditemukan", nil)
		return
	}

	result, err := h.service.GetSession(r.Context(), principal.UserID)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Terjadi kesalahan yang tidak terduga")
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
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi login tidak ditemukan", nil)
		return
	}

	var input dto.ChangePasswordRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	if err := h.service.ChangePassword(r.Context(), principal.UserID, input.CurrentPassword, input.NewPassword); err != nil {
		h.writeAuthError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLogWithUser(r.Context(), principal.UserID, "update", "admin", "password", principal.UserID, nil, map[string]any{
		"changed": true,
	})

	response.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Kata sandi berhasil diubah. Semua sesi aktif telah dicabut.",
	}, nil)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var input dto.RegisterRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.Register(r.Context(), input, r.UserAgent(), clientIP(r))
	if err != nil {
		// Audit failed attempts (no user ID available).
		reason := ""
		switch {
		case errors.Is(err, authservice.ErrRegistrationDisabled):
			reason = "registration_disabled"
		case errors.Is(err, authservice.ErrRegistrationCodeMissing):
			reason = "code_missing"
		case errors.Is(err, authservice.ErrRegistrationCodeExpired):
			reason = "code_expired"
		case errors.Is(err, authservice.ErrRegistrationCodeInvalid):
			reason = "code_invalid"
		case errors.Is(err, authservice.ErrRegistrationDomainDeny):
			reason = "domain_denied"
		}
		if reason != "" {
			platformmiddleware.AuditLog(r.Context(), "register_denied", "admin", "auth", "register", nil, map[string]any{
				"email":  strings.ToLower(strings.TrimSpace(input.Email)),
				"reason": reason,
			})
		}
		h.writeAuthError(r.Context(), w, err)
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
		reason := loginFailureReason(err)
		if reason != "" {
			platformmiddleware.AuditLog(r.Context(), "login_failed", "admin", "auth", "login", nil, map[string]any{
				"email":  strings.ToLower(strings.TrimSpace(input.Email)),
				"reason": reason,
			})
		}
		h.writeAuthError(r.Context(), w, err)
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
	// /auth/refresh authenticates with a SameSite=Lax HTTP-only cookie. Lax
	// blocks cross-site form posts but not top-level navigations and not
	// custom-content-type submissions, so we add a CSRF guard: the request
	// must carry a custom header that browsers cannot set on cross-origin
	// non-fetch submissions without a CORS preflight.
	if !hasCSRFHeader(r) {
		response.WriteError(w, http.StatusForbidden, "CSRF_REQUIRED", "X-Requested-With header is required", nil)
		return
	}

	refreshToken, err := h.readRefreshTokenCookie(r)
	if err != nil {
		platformmiddleware.AuditLog(r.Context(), "token_refresh_failed", "admin", "auth", "refresh", nil, map[string]any{
			"reason": "cookie_missing",
		})
		response.WriteError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Cookie refresh token tidak ditemukan", nil)
		return
	}

	result, err := h.service.Refresh(r.Context(), refreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		reason := "unknown"
		switch {
		case errors.Is(err, authservice.ErrInvalidRefreshToken):
			reason = "invalid_token"
		case errors.Is(err, authservice.ErrExpiredRefreshToken):
			reason = "expired_token"
		case errors.Is(err, authservice.ErrInactiveUser):
			reason = "inactive_user"
		}
		platformmiddleware.AuditLog(r.Context(), "token_refresh_failed", "admin", "auth", "refresh", nil, map[string]any{
			"reason": reason,
		})
		h.writeAuthError(r.Context(), w, err)
		return
	}

	h.setRefreshTokenCookie(w, result.Tokens.RefreshToken)
	platformmiddleware.AuditLogWithUser(r.Context(), result.User.ID, "token_refresh", "admin", "auth", result.User.ID, nil, map[string]any{
		"rotated": true,
	})
	response.WriteJSON(w, http.StatusOK, dto.AuthResponse{
		User:         result.User,
		ModuleRoles:  result.ModuleRoles,
		Permissions:  result.Permissions,
		IsSuperAdmin: result.IsSuperAdmin,
		Tokens:       result.Tokens,
	}, nil)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	// Logout is not destructive enough to be a real CSRF target, but it does
	// rely on the same cookie as /refresh. Apply the same custom-header
	// guard so an attacker cannot remotely sign a victim out either.
	if !hasCSRFHeader(r) {
		response.WriteError(w, http.StatusForbidden, "CSRF_REQUIRED", "X-Requested-With header is required", nil)
		return
	}

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

func (h *Handler) publicOptions(w http.ResponseWriter, r *http.Request) {
	options, err := h.service.GetPublicAuthOptions(r.Context())
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Gagal memuat opsi auth tenant")
		return
	}

	response.WriteJSON(w, http.StatusOK, options, nil)
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

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var input dto.ForgotPasswordRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(input.Email))
	if err := h.service.RequestPasswordReset(r.Context(), input.Email, authservice.PasswordResetRequestMeta{
		PublicBaseURL: requestPublicBaseURL(r, h.cookieSecure),
		UserAgent:     r.UserAgent(),
		IPAddress:     clientIP(r),
	}); err != nil {
		platformmiddleware.AuditLog(r.Context(), "password_reset_failed", "admin", "auth", "forgot_password", nil, map[string]any{
			"email": normalizedEmail,
		})
		h.writeAuthError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "password_reset_requested", "admin", "auth", "forgot_password", nil, map[string]any{
		"email": normalizedEmail,
	})
	response.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Jika email ditemukan pada tenant ini, link reset kata sandi akan dikirim.",
	}, nil)
}

func (h *Handler) validateResetPassword(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Token reset wajib diisi", map[string]string{"token": "required"})
		return
	}

	if err := h.service.ValidatePasswordResetToken(r.Context(), token); err != nil {
		h.writeAuthError(r.Context(), w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]bool{"valid": true}, nil)
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var input dto.ResetPasswordRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	userID, err := h.service.ResetPasswordWithToken(r.Context(), input.Token, input.NewPassword)
	if err != nil {
		h.writeAuthError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLogWithUser(r.Context(), userID, "reset_password", "admin", "auth", userID, nil, map[string]any{
		"reset_via_email": true,
	})

	response.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Kata sandi berhasil diatur ulang. Silakan masuk dengan kata sandi baru.",
	}, nil)
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
	return httputil.DecodeAndValidate(h.validator, w, r, target)
}

func (h *Handler) writeAuthError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authservice.ErrEmailAlreadyExists):
		response.WriteError(w, http.StatusConflict, "EMAIL_ALREADY_EXISTS", err.Error(), nil)
	case errors.Is(err, authservice.ErrInvalidCredentials):
		response.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", err.Error(), nil)
	case errors.Is(err, authservice.ErrInvalidCurrentPassword):
		response.WriteError(w, http.StatusBadRequest, "INVALID_CURRENT_PASSWORD", err.Error(), nil)
	case errors.Is(err, authservice.ErrInactiveUser):
		response.WriteError(w, http.StatusForbidden, "INACTIVE_USER", err.Error(), nil)
	case errors.Is(err, authservice.ErrAccountLocked):
		response.WriteError(w, http.StatusTooManyRequests, "ACCOUNT_LOCKED", err.Error(), nil)
	case errors.Is(err, authservice.ErrInvalidRefreshToken), errors.Is(err, authservice.ErrExpiredRefreshToken):
		response.WriteError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", err.Error(), nil)
	case errors.Is(err, authservice.ErrInvalidResetToken):
		response.WriteError(w, http.StatusBadRequest, "INVALID_RESET_TOKEN", err.Error(), nil)
	case errors.Is(err, authservice.ErrExpiredResetToken):
		response.WriteError(w, http.StatusBadRequest, "EXPIRED_RESET_TOKEN", err.Error(), nil)
	case errors.Is(err, authservice.ErrPasswordResetDisabled):
		response.WriteError(w, http.StatusServiceUnavailable, "PASSWORD_RESET_DISABLED", err.Error(), nil)
	case errors.Is(err, authservice.ErrPasswordUnchanged):
		response.WriteError(w, http.StatusBadRequest, "PASSWORD_UNCHANGED", err.Error(), nil)
	case errors.Is(err, authservice.ErrRegistrationDisabled):
		response.WriteError(w, http.StatusForbidden, "REGISTRATION_DISABLED", err.Error(), nil)
	case errors.Is(err, authservice.ErrRegistrationCodeMissing):
		response.WriteError(w, http.StatusBadRequest, "REGISTRATION_CODE_REQUIRED", err.Error(), map[string]string{"registration_code": "required"})
	case errors.Is(err, authservice.ErrRegistrationCodeExpired):
		response.WriteError(w, http.StatusForbidden, "REGISTRATION_CODE_EXPIRED", err.Error(), nil)
	case errors.Is(err, authservice.ErrRegistrationCodeInvalid):
		response.WriteError(w, http.StatusForbidden, "REGISTRATION_CODE_INVALID", err.Error(), nil)
	case errors.Is(err, authservice.ErrRegistrationDomainDeny):
		response.WriteError(w, http.StatusForbidden, "REGISTRATION_DOMAIN_DENIED", err.Error(), map[string]string{"email": err.Error()})
	default:
		response.WriteInternalError(ctx, w, err, "Terjadi kesalahan yang tidak terduga")
	}
}

func validationDetails(err error) map[string]string {
	return httputil.ValidationDetails(err)
}

// loginFailureReason maps a service-layer auth error to a short audit code.
// Errors that don't represent a real authentication failure (validation,
// internal) return "" so the caller skips the audit entry.
func loginFailureReason(err error) string {
	switch {
	case errors.Is(err, authservice.ErrInvalidCredentials):
		return "invalid_credentials"
	case errors.Is(err, authservice.ErrInactiveUser):
		return "inactive_user"
	case errors.Is(err, authservice.ErrAccountLocked):
		return "account_locked"
	default:
		return ""
	}
}

// hasCSRFHeader returns true when the caller proved the request originated
// from a same-origin script. Browsers refuse to set custom headers on
// cross-site form posts and image/iframe loads without a CORS preflight, so
// the presence of either X-Requested-With or X-Csrf-Token is enough to
// distinguish a legitimate fetch() from a CSRF-style submission.
func hasCSRFHeader(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("X-Requested-With")) != "" {
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Csrf-Token")) != "" {
		return true
	}
	return false
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}

func requestPublicBaseURL(r *http.Request, secureCookie bool) string {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}

	forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	switch {
	case forwardedProto != "":
		return fmt.Sprintf("%s://%s", forwardedProto, host)
	case r.TLS != nil:
		return fmt.Sprintf("https://%s", host)
	case secureCookie:
		return fmt.Sprintf("https://%s", host)
	case strings.Contains(strings.ToLower(host), "localhost"):
		return fmt.Sprintf("http://%s", host)
	case strings.Contains(host, ".local"):
		return fmt.Sprintf("http://%s", host)
	default:
		return fmt.Sprintf("https://%s", host)
	}
}
