package hris

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

type EmployeesHandler struct {
	service      *hrisservice.EmployeesService
	compensation *hrisservice.CompensationService
	users        exportutil.UserLookup
	validator    *validator.Validate
	uploadsDir   string
}

func NewEmployeesHandler(
	service *hrisservice.EmployeesService,
	compensation *hrisservice.CompensationService,
	uploadsDir string,
	users exportutil.UserLookup,
) *EmployeesHandler {
	return &EmployeesHandler{
		service:      service,
		compensation: compensation,
		users:        users,
		validator:    newValidator(),
		uploadsDir:   uploadsDir,
	}
}

func (h *EmployeesHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("hris:employee:create")).Post("/", h.createEmployee)
	router.With(platformmiddleware.RequirePermission("hris:employee:view")).Get("/", h.listEmployees)
	router.With(platformmiddleware.RequirePermission("hris:employee:view")).Get("/export", h.exportList)
	router.With(platformmiddleware.RequirePermission("hris:employee:view")).Get("/{employeeID}", h.getEmployee)
	router.With(platformmiddleware.RequireAllPermissions("hris:employee:view", "hris:salary:view")).Get("/{employeeID}/export", h.exportDetail)
	router.With(platformmiddleware.RequirePermission("hris:employee:edit")).Put("/{employeeID}", h.updateEmployee)
	router.With(platformmiddleware.RequirePermission("hris:employee:edit")).Post("/{employeeID}/avatar", h.uploadAvatar)
	router.With(platformmiddleware.RequirePermission("hris:employee:delete")).Delete("/{employeeID}", h.deleteEmployee)
}

func (h *EmployeesHandler) createEmployee(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.CreateEmployeeRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	result, err := h.service.CreateEmployee(r.Context(), input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "hris", "employee", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *EmployeesHandler) listEmployees(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}

	result, total, page, perPage, err := h.service.ListEmployees(r.Context(), query)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *EmployeesHandler) getEmployee(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetEmployee(r.Context(), chi.URLParam(r, "employeeID"))
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *EmployeesHandler) updateEmployee(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.UpdateEmployeeRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	employeeID := chi.URLParam(r, "employeeID")
	result, err := h.service.UpdateEmployee(r.Context(), employeeID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "hris", "employee", employeeID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *EmployeesHandler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "Avatar upload must use multipart form data", nil)
		return
	}

	var fileHeader *multipart.FileHeader
	if files := r.MultipartForm.File["avatar"]; len(files) > 0 {
		fileHeader = files[0]
	} else if files := r.MultipartForm.File["file"]; len(files) > 0 {
		fileHeader = files[0]
	}

	if fileHeader == nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Avatar image is required", map[string]string{"avatar": "required"})
		return
	}

	employeeID := chi.URLParam(r, "employeeID")
	current, err := h.service.GetEmployee(r.Context(), employeeID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	avatarPath, err := saveEmployeeAvatar(h.uploadsDir, employeeID, fileHeader)
	if err != nil {
		switch {
		case errors.Is(err, errEmployeeAvatarValidation):
			response.WriteError(w, http.StatusBadRequest, "AVATAR_UPLOAD_FAILED", err.Error(), nil)
		default:
			response.WriteError(w, http.StatusInternalServerError, "AVATAR_STORAGE_FAILED", "Avatar storage is not available right now", nil)
		}
		return
	}

	result, err := h.service.UpdateEmployeeAvatar(r.Context(), employeeID, avatarPath)
	if err != nil {
		_ = os.Remove(filepath.Join(h.uploadsDir, filepath.FromSlash(avatarPath)))
		h.writeError(w, err)
		return
	}

	if current.AvatarURL != nil && strings.TrimSpace(*current.AvatarURL) != "" {
		oldPath := filepath.ToSlash(strings.TrimSpace(*current.AvatarURL))
		if oldPath != avatarPath && isEmployeeAvatarPath(oldPath, employeeID) {
			_ = os.Remove(filepath.Join(h.uploadsDir, filepath.FromSlash(oldPath)))
		}
	}

	platformmiddleware.AuditLog(r.Context(), "update", "hris", "employee_avatar", employeeID, map[string]any{
		"avatar_url": current.AvatarURL,
	}, map[string]any{
		"avatar_url": result.AvatarURL,
	})
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *EmployeesHandler) deleteEmployee(w http.ResponseWriter, r *http.Request) {
	employeeID := chi.URLParam(r, "employeeID")
	if err := h.service.DeleteEmployee(r.Context(), employeeID); err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "employee", employeeID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Employee deleted successfully"}, nil)
}

func (h *EmployeesHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (hrisdto.ListEmployeesQuery, bool) {
	query := hrisdto.ListEmployeesQuery{
		Search:           r.URL.Query().Get("search"),
		Department:       r.URL.Query().Get("department"),
		EmploymentStatus: r.URL.Query().Get("status"),
	}

	if pageRaw := r.URL.Query().Get("page"); pageRaw != "" {
		page, err := strconv.Atoi(pageRaw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return hrisdto.ListEmployeesQuery{}, false
		}
		query.Page = page
	}

	if perPageRaw := r.URL.Query().Get("per_page"); perPageRaw != "" {
		perPage, err := strconv.Atoi(perPageRaw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return hrisdto.ListEmployeesQuery{}, false
		}
		query.PerPage = perPage
	}

	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return hrisdto.ListEmployeesQuery{}, false
	}

	return query, true
}

func (h *EmployeesHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrEmployeeEmailExists):
		response.WriteError(w, http.StatusConflict, "EMPLOYEE_EMAIL_EXISTS", err.Error(), map[string]string{"email": "already exists"})
	case errors.Is(err, hrisservice.ErrEmployeeUserLinkedTwice):
		response.WriteError(w, http.StatusConflict, "EMPLOYEE_USER_ALREADY_LINKED", err.Error(), map[string]string{"user_id": "already linked"})
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}

var (
	errEmployeeAvatarValidation = errors.New("employee avatar validation failed")
	errEmployeeAvatarStorage    = errors.New("employee avatar storage failed")
)

func saveEmployeeAvatar(baseUploadsDir string, employeeID string, file *multipart.FileHeader) (string, error) {
	if file.Size > 5<<20 {
		return "", fmt.Errorf("%w: avatar image must be smaller than 5MB", errEmployeeAvatarValidation)
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarStorage, err)
	}
	defer src.Close()

	sniff := make([]byte, 512)
	n, readErr := src.Read(sniff)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarStorage, readErr)
	}
	if _, err := src.Seek(0, 0); err != nil {
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarStorage, err)
	}

	contentType := http.DetectContentType(sniff[:n])
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("%w: avatar must be an image file", errEmployeeAvatarValidation)
	}

	dir := filepath.Join(baseUploadsDir, "employees", employeeID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarStorage, err)
	}

	filename := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + sanitizeEmployeeAvatarFilename(file.Filename)
	destinationPath := filepath.Join(dir, filename)

	dst, err := os.Create(destinationPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarStorage, err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(destinationPath)
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarStorage, err)
	}

	if err := dst.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarStorage, err)
	}

	return filepath.ToSlash(filepath.Join("employees", employeeID, filename)), nil
}

func sanitizeEmployeeAvatarFilename(value string) string {
	filename := strings.ToLower(strings.TrimSpace(value))
	filename = strings.ReplaceAll(filename, " ", "-")
	filename = strings.ReplaceAll(filename, "..", "")
	return filename
}

func isEmployeeAvatarPath(path string, employeeID string) bool {
	return strings.HasPrefix(filepath.ToSlash(path), "employees/"+employeeID+"/")
}
