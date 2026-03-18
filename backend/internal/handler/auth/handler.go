package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
)

type Handler struct {
	service   *authservice.Service
	validator *validator.Validate
}

func New(service *authservice.Service) *Handler {
	return &Handler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
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
		"user":        result.User,
		"roles":       result.Roles,
		"permissions": result.Permissions,
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

	response.WriteJSON(w, http.StatusCreated, dto.AuthResponse{
		User:        result.User,
		Roles:       result.Roles,
		Permissions: result.Permissions,
		Tokens:      result.Tokens,
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

	response.WriteJSON(w, http.StatusOK, dto.AuthResponse{
		User:        result.User,
		Roles:       result.Roles,
		Permissions: result.Permissions,
		Tokens:      result.Tokens,
	}, nil)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var input dto.RefreshRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.Refresh(r.Context(), input.RefreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, dto.AuthResponse{
		User:        result.User,
		Roles:       result.Roles,
		Permissions: result.Permissions,
		Tokens:      result.Tokens,
	}, nil)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var input dto.LogoutRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	if err := h.service.Logout(r.Context(), input.RefreshToken); err != nil {
		h.writeAuthError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true}, nil)
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
	case errors.Is(err, authservice.ErrInvalidRefreshToken), errors.Is(err, authservice.ErrExpiredRefreshToken):
		response.WriteError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", err.Error(), nil)
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
