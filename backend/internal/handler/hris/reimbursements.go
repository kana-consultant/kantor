package hris

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
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

type ReimbursementsHandler struct {
	service    *hrisservice.ReimbursementsService
	users      exportutil.UserLookup
	validator  *validator.Validate
	uploadsDir string
}

func NewReimbursementsHandler(service *hrisservice.ReimbursementsService, uploadsDir string, users exportutil.UserLookup) *ReimbursementsHandler {
	return &ReimbursementsHandler{
		service:    service,
		users:      users,
		validator:  newValidator(),
		uploadsDir: uploadsDir,
	}
}

func (h *ReimbursementsHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("hris:reimbursement:create")).Post("/", h.create)
	router.With(platformmiddleware.RequirePermission("hris:reimbursement:view")).Get("/", h.list)
	router.With(platformmiddleware.RequirePermission("hris:reimbursement:view")).Get("/export", h.export)
	router.With(platformmiddleware.RequirePermission("hris:reimbursement:view")).Get("/summary", h.summary)
	router.With(platformmiddleware.RequirePermission("hris:reimbursement:view")).Get("/{reimbursementID}", h.get)
	router.With(platformmiddleware.RequirePermission("hris:reimbursement:edit")).Post("/{reimbursementID}/attachments", h.uploadAttachments)
	router.With(platformmiddleware.RequirePermission("hris:reimbursement:approve")).Patch("/{reimbursementID}/review", h.review)
	router.With(platformmiddleware.RequirePermission("hris:reimbursement:mark_paid")).Patch("/{reimbursementID}/mark-paid", h.markPaid)
}

func (h *ReimbursementsHandler) create(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input hrisdto.CreateReimbursementRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	result, err := h.service.Create(r.Context(), input, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "reimbursement", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *ReimbursementsHandler) list(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}

	items, total, page, perPage, err := h.service.List(r.Context(), query, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *ReimbursementsHandler) get(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	item, err := h.service.Get(r.Context(), chi.URLParam(r, "reimbursementID"), principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *ReimbursementsHandler) uploadAttachments(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "Attachment upload must use multipart form data", nil)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "At least one file is required", map[string]string{"files": "required"})
		return
	}

	paths, err := saveAttachments(h.uploadsDir, files)
	if err != nil {
		switch {
		case errors.Is(err, errAttachmentValidation):
			response.WriteError(w, http.StatusBadRequest, "ATTACHMENT_UPLOAD_FAILED", err.Error(), nil)
		default:
			response.WriteError(w, http.StatusInternalServerError, "ATTACHMENT_STORAGE_FAILED", "Attachment storage is not available right now", nil)
		}
		return
	}

	reimbursementID := chi.URLParam(r, "reimbursementID")
	item, err := h.service.AddAttachments(r.Context(), reimbursementID, paths, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "upload_attachments", "hris", "reimbursement", reimbursementID, nil, nil)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *ReimbursementsHandler) markPaid(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input hrisdto.MarkPaidRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	reimbursementID := chi.URLParam(r, "reimbursementID")
	item, err := h.service.MarkPaid(r.Context(), reimbursementID, input.Notes, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "mark_paid", "hris", "reimbursement", reimbursementID, nil, input)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *ReimbursementsHandler) summary(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	month, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("month")))
	year, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("year")))
	item, err := h.service.Summary(r.Context(), month, year, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *ReimbursementsHandler) review(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input hrisdto.ReviewReimbursementRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	reimbursementID := chi.URLParam(r, "reimbursementID")
	item, err := h.service.ManagerReview(r.Context(), reimbursementID, input, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "review", "hris", "reimbursement", reimbursementID, nil, input)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *ReimbursementsHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (hrisdto.ListReimbursementsQuery, bool) {
	query := hrisdto.ListReimbursementsQuery{
		Status:     r.URL.Query().Get("status"),
		EmployeeID: r.URL.Query().Get("employee"),
	}
	if value := strings.TrimSpace(r.URL.Query().Get("page")); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return hrisdto.ListReimbursementsQuery{}, false
		}
		query.Page = page
	}
	if value := strings.TrimSpace(r.URL.Query().Get("per_page")); value != "" {
		perPage, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return hrisdto.ListReimbursementsQuery{}, false
		}
		query.PerPage = perPage
	}
	if value := strings.TrimSpace(r.URL.Query().Get("month")); value != "" {
		month, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"month": "must be a number"})
			return hrisdto.ListReimbursementsQuery{}, false
		}
		query.Month = month
	}
	if value := strings.TrimSpace(r.URL.Query().Get("year")); value != "" {
		year, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"year": "must be a number"})
			return hrisdto.ListReimbursementsQuery{}, false
		}
		query.Year = year
	}
	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return hrisdto.ListReimbursementsQuery{}, false
	}
	return query, true
}

func (h *ReimbursementsHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrReimbursementNotFound):
		response.WriteError(w, http.StatusNotFound, "REIMBURSEMENT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrReimbursementForbidden):
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"employee_id": "not found"})
	default:
		slog.Error("unexpected reimbursement error", "error", err)
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}

var (
	errAttachmentValidation = errors.New("attachment validation failed")
	errAttachmentStorage    = errors.New("attachment storage failed")
)

func saveAttachments(baseUploadsDir string, files []*multipart.FileHeader) ([]string, error) {
	dir := filepath.Join(baseUploadsDir, "reimbursements")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: %w", errAttachmentStorage, err)
	}

	paths := make([]string, 0, len(files))
	for _, file := range files {
		if file.Size > 10<<20 {
			return nil, fmt.Errorf("%w: each file must be smaller than 10MB", errAttachmentValidation)
		}

		src, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errAttachmentStorage, err)
		}

		sniff := make([]byte, 512)
		n, readErr := src.Read(sniff)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = src.Close()
			return nil, fmt.Errorf("%w: %w", errAttachmentStorage, readErr)
		}
		if _, err := src.Seek(0, 0); err != nil {
			_ = src.Close()
			return nil, fmt.Errorf("%w: %w", errAttachmentStorage, err)
		}

		contentType := http.DetectContentType(sniff[:n])
		if !allowedAttachmentType(contentType) {
			_ = src.Close()
			return nil, fmt.Errorf("%w: only image and PDF files are allowed", errAttachmentValidation)
		}

		filename := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + sanitizeFilename(file.Filename)
		destinationPath := filepath.Join(dir, filename)
		dst, err := os.Create(destinationPath)
		if err != nil {
			_ = src.Close()
			return nil, fmt.Errorf("%w: %w", errAttachmentStorage, err)
		}
		if _, err := io.Copy(dst, src); err != nil {
			_ = dst.Close()
			_ = src.Close()
			_ = os.Remove(destinationPath)
			return nil, fmt.Errorf("%w: %w", errAttachmentStorage, err)
		}
		_ = dst.Close()
		_ = src.Close()
		paths = append(paths, filepath.ToSlash(filepath.Join("reimbursements", filename)))
	}

	return paths, nil
}

func allowedAttachmentType(contentType string) bool {
	return strings.HasPrefix(contentType, "image/") || contentType == "application/pdf"
}

func sanitizeFilename(value string) string {
	filename := strings.ToLower(strings.TrimSpace(value))
	filename = strings.ReplaceAll(filename, " ", "-")
	filename = strings.ReplaceAll(filename, "..", "")
	return filename
}
