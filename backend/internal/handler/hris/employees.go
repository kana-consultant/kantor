package hris

import (
	"context"
	"encoding/json"
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

	"github.com/kana-consultant/kantor/backend/internal/clientvia"
	"github.com/kana-consultant/kantor/backend/internal/docgen"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
	"github.com/kana-consultant/kantor/backend/internal/uploads"
)

type EmployeesHandler struct {
	service      *hrisservice.EmployeesService
	compensation *hrisservice.CompensationService
	users        exportutil.UserLookup
	validator    *validator.Validate
	uploadsDir   string
}

const maxEmployeeAvatarMultipartMaxBytes = 6 << 20

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
	router.Get("/me", h.getMyEmployee)
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
		h.writeError(r.Context(), w, err)
		return
	}

	auditInput := input
	auditInput.BankAccountNumber = maskedAccountForAudit(input.BankAccountNumber)
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "employee", result.ID, nil, auditInput)
	response.WriteJSON(w, http.StatusCreated, h.visibleEmployee(r, result), nil)
}

func (h *EmployeesHandler) listEmployees(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}

	result, total, page, perPage, err := h.service.ListEmployees(r.Context(), query)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	for index := range result {
		result[index] = h.visibleEmployee(r, result[index])
	}

	response.WriteJSON(w, http.StatusOK, result, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *EmployeesHandler) getMyEmployee(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.GetMyEmployee(r.Context(), principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *EmployeesHandler) getEmployee(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetEmployee(r.Context(), chi.URLParam(r, "employeeID"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, h.visibleEmployee(r, result), nil)
}

func (h *EmployeesHandler) updateEmployee(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.UpdateEmployeeRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	employeeID := chi.URLParam(r, "employeeID")
	// The stored record tells which fields this save changes: the audit
	// denylist redacts the bank account number, so without it a new number
	// and an unchanged one would look the same.
	previous, previousErr := h.service.GetEmployee(r.Context(), employeeID)
	if clientvia.IsMCP(r) {
		// Fail closed: without the stored record the check below cannot run.
		if previousErr != nil {
			h.writeError(r.Context(), w, previousErr)
			return
		}
		if employeeEmailChanges(previous, input) {
			response.WriteError(w, http.StatusConflict, "EMPLOYEE_EMAIL_LOCKED", "Lewat MCP, email karyawan tidak dapat diubah; ubah di aplikasi web.", map[string]string{"email": "mcp"})
			return
		}
	}
	result, err := h.service.UpdateEmployee(r.Context(), employeeID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	auditInput := input
	auditInput.BankAccountNumber = maskedAccountForAudit(input.BankAccountNumber)
	var auditValue any = auditInput
	if previousErr == nil {
		auditValue = withChangedFields(auditInput, changedEmployeeFields(previous, result))
	}
	platformmiddleware.AuditLog(r.Context(), "update", "hris", "employee", employeeID, nil, auditValue)
	response.WriteJSON(w, http.StatusOK, h.visibleEmployee(r, result), nil)
}

// employeeEmailChanges reports whether an update would change the
// employee's e-mail. Through the MCP tool surface that is refused: for an
// employee without an account, employees.email decides which user account
// the employee gets linked to, and document e-mail goes to that account.
func employeeEmailChanges(previous model.Employee, input hrisdto.UpdateEmployeeRequest) bool {
	return !strings.EqualFold(strings.TrimSpace(previous.Email), strings.TrimSpace(input.Email))
}

// changedEmployeeFields lists the fields (names only, never values) that
// differ between two versions of an employee record.
func changedEmployeeFields(previous model.Employee, next model.Employee) []string {
	optional := func(value *string) string {
		if value == nil {
			return ""
		}
		return strings.TrimSpace(*value)
	}
	checks := []struct {
		name    string
		changed bool
	}{
		{"full_name", previous.FullName != next.FullName},
		{"email", !strings.EqualFold(previous.Email, next.Email)},
		{"phone", optional(previous.Phone) != optional(next.Phone)},
		{"position", previous.Position != next.Position},
		{"department", optional(previous.Department) != optional(next.Department)},
		{"date_joined", !previous.DateJoined.Equal(next.DateJoined)},
		{"employment_status", previous.EmploymentStatus != next.EmploymentStatus},
		{"address", optional(previous.Address) != optional(next.Address)},
		{"emergency_contact", optional(previous.EmergencyContact) != optional(next.EmergencyContact)},
		{"avatar_url", optional(previous.AvatarURL) != optional(next.AvatarURL)},
		{"bank_account_number", optional(previous.BankAccountNumber) != optional(next.BankAccountNumber)},
		{"bank_name", optional(previous.BankName) != optional(next.BankName)},
		{"linkedin_profile", optional(previous.LinkedInProfile) != optional(next.LinkedInProfile)},
		{"ssh_keys", optional(previous.SSHKeys) != optional(next.SSHKeys)},
	}
	fields := make([]string, 0, len(checks))
	for _, check := range checks {
		if check.changed {
			fields = append(fields, check.name)
		}
	}
	return fields
}

// withChangedFields returns the audit value as a JSON object with a
// changed_fields list added.
func withChangedFields(value any, fields []string) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	object := map[string]any{}
	if err := json.Unmarshal(raw, &object); err != nil {
		return value
	}
	object["changed_fields"] = fields
	return object
}

func (h *EmployeesHandler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEmployeeAvatarMultipartMaxBytes)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		if platformmiddleware.IsBodyTooLargeError(err) {
			platformmiddleware.WriteBodyTooLargeError(w)
			return
		}
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
		h.writeError(r.Context(), w, err)
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
		h.writeError(r.Context(), w, err)
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
	response.WriteJSON(w, http.StatusOK, h.visibleEmployee(r, result), nil)
}

func (h *EmployeesHandler) deleteEmployee(w http.ResponseWriter, r *http.Request) {
	employeeID := chi.URLParam(r, "employeeID")
	if err := h.service.DeleteEmployee(r.Context(), employeeID); err != nil {
		h.writeError(r.Context(), w, err)
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

func (h *EmployeesHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrEmployeeHasDocuments):
		response.WriteError(w, http.StatusConflict, "EMPLOYEE_HAS_LEGAL_RECORDS", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrEmployeeLinkedEmailLocked):
		response.WriteError(w, http.StatusConflict, "EMPLOYEE_EMAIL_LOCKED", err.Error(), map[string]string{"email": "locked"})
	case errors.Is(err, hrisservice.ErrEmployeeEmailExists):
		response.WriteError(w, http.StatusConflict, "EMPLOYEE_EMAIL_EXISTS", err.Error(), map[string]string{"email": "already exists"})
	case errors.Is(err, hrisservice.ErrEmployeeUserLinkedTwice):
		response.WriteError(w, http.StatusConflict, "EMPLOYEE_USER_ALREADY_LINKED", err.Error(), map[string]string{"user_id": "already linked"})
	default:
		response.WriteInternalError(ctx, w, err, "An unexpected error occurred")
	}
}

var (
	errEmployeeAvatarValidation = errors.New("employee avatar validation failed")
	errEmployeeAvatarStorage    = errors.New("employee avatar storage failed")
)

func saveEmployeeAvatar(baseUploadsDir string, employeeID string, file *multipart.FileHeader) (string, error) {
	if _, err := uploads.ValidateMultipartFile(uploads.KindAvatar, file); err != nil {
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarValidation, err)
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("%w: %w", errEmployeeAvatarStorage, err)
	}
	defer src.Close()

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

// visibleEmployee masks the bank account number ('******7890') unless the
// caller holds hris:employee_identity:view or the record is their own. MCP
// tools go through these handlers and inherit the masking.
func (h *EmployeesHandler) visibleEmployee(r *http.Request, employee model.Employee) model.Employee {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if ok && canSeeFullBankAccount(principal, employee) {
		return employee
	}
	return hrisservice.MaskEmployeeBankAccount(employee)
}

func canSeeFullBankAccount(principal platformmiddleware.Principal, employee model.Employee) bool {
	if principal.IsSuperAdmin {
		return true
	}
	if employee.UserID != nil && principal.UserID != "" && *employee.UserID == principal.UserID {
		return true
	}
	if principal.Cached != nil && principal.Cached.Permissions[permissionIdentityView] {
		return true
	}
	for _, item := range principal.Permissions {
		if item == permissionIdentityView {
			return true
		}
	}
	return false
}

// maskedAccountForAudit keeps the full account number out of audit rows.
func maskedAccountForAudit(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" || hrisservice.IsMaskedBankAccount(value) {
		return value
	}
	masked := docgen.MaskAccount(*value)
	return &masked
}
