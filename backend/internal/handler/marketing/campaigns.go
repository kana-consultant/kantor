package marketing

import (
	"context"
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
	"github.com/google/uuid"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	"github.com/kana-consultant/kantor/backend/internal/response"
	marketingservice "github.com/kana-consultant/kantor/backend/internal/service/marketing"
	"github.com/kana-consultant/kantor/backend/internal/uploads"
)

type CampaignsHandler struct {
	service    *marketingservice.CampaignsService
	users      exportutil.UserLookup
	validator  *validator.Validate
	uploadsDir string
}

func NewCampaignsHandler(service *marketingservice.CampaignsService, uploadsDir string, users exportutil.UserLookup) *CampaignsHandler {
	return &CampaignsHandler{
		service:    service,
		users:      users,
		validator:  newValidator(),
		uploadsDir: uploadsDir,
	}
}

func (h *CampaignsHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("marketing:campaign:create")).Post("/", h.createCampaign)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:view")).Get("/", h.listCampaigns)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:view")).Get("/export", h.export)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:view")).Get("/kanban", h.kanban)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:view")).Get("/{campaignID}", h.getCampaign)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:view")).Get("/{campaignID}/activities", h.listActivities)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:edit")).Put("/{campaignID}", h.updateCampaign)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:delete")).Delete("/{campaignID}", h.deleteCampaign)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:edit")).Patch("/{campaignID}/move", h.moveCampaign)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:edit")).Post("/{campaignID}/attachments", h.uploadAttachment)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:view")).Get("/{campaignID}/attachments", h.listAttachments)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:delete")).Delete("/{campaignID}/attachments/{attachmentID}", h.deleteAttachment)
}

func (h *CampaignsHandler) RegisterColumnRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("marketing:campaign:view")).Get("/", h.listColumns)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:manage_columns")).Post("/", h.createColumn)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:manage_columns")).Put("/{columnID}", h.updateColumn)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:manage_columns")).Delete("/{columnID}", h.deleteColumn)
	router.With(platformmiddleware.RequirePermission("marketing:campaign:manage_columns")).Patch("/reorder", h.reorderColumns)
}

func (h *CampaignsHandler) createCampaign(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.CreateCampaignRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	item, err := h.service.CreateCampaign(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "marketing", "campaign", item.Campaign.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, item, nil)
}

func (h *CampaignsHandler) listCampaigns(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}

	items, total, page, perPage, err := h.service.ListCampaigns(r.Context(), query)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, items, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *CampaignsHandler) kanban(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListKanban(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *CampaignsHandler) getCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID, ok := validateCampaignUUIDParam(w, "campaignID", chi.URLParam(r, "campaignID"))
	if !ok {
		return
	}
	item, err := h.service.GetCampaign(r.Context(), campaignID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *CampaignsHandler) listActivities(w http.ResponseWriter, r *http.Request) {
	campaignID, ok := validateCampaignUUIDParam(w, "campaignID", chi.URLParam(r, "campaignID"))
	if !ok {
		return
	}
	items, err := h.service.ListActivities(r.Context(), campaignID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *CampaignsHandler) updateCampaign(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.UpdateCampaignRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	campaignID, ok := validateCampaignUUIDParam(w, "campaignID", chi.URLParam(r, "campaignID"))
	if !ok {
		return
	}
	item, err := h.service.UpdateCampaign(r.Context(), campaignID, input, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "marketing", "campaign", campaignID, nil, input)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *CampaignsHandler) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID, ok := validateCampaignUUIDParam(w, "campaignID", chi.URLParam(r, "campaignID"))
	if !ok {
		return
	}
	if err := h.service.DeleteCampaign(r.Context(), campaignID); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "marketing", "campaign", campaignID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Campaign deleted successfully"}, nil)
}

func (h *CampaignsHandler) moveCampaign(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.MoveCampaignRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	campaignID, ok := validateCampaignUUIDParam(w, "campaignID", chi.URLParam(r, "campaignID"))
	if !ok {
		return
	}
	item, err := h.service.MoveCampaign(r.Context(), campaignID, input, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "move", "marketing", "campaign", campaignID, nil, input)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *CampaignsHandler) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "Attachment upload must use multipart form data", nil)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "At least one file is required", map[string]string{"files": "required"})
		return
	}

	uploadedFiles, err := saveCampaignAttachments(h.uploadsDir, files)
	if err != nil {
		switch {
		case errors.Is(err, errCampaignAttachmentValidation):
			response.WriteError(w, http.StatusBadRequest, "ATTACHMENT_UPLOAD_FAILED", err.Error(), nil)
		default:
			response.WriteError(w, http.StatusInternalServerError, "ATTACHMENT_STORAGE_FAILED", "Attachment storage is not available right now", nil)
		}
		return
	}

	campaignID, ok := validateCampaignUUIDParam(w, "campaignID", chi.URLParam(r, "campaignID"))
	if !ok {
		for _, uploaded := range uploadedFiles {
			_ = os.Remove(filepath.Join(h.uploadsDir, filepath.FromSlash(uploaded.FilePath)))
		}
		return
	}
	params := make([]marketingrepo.CreateCampaignAttachmentParams, 0, len(uploadedFiles))
	for _, file := range uploadedFiles {
		params = append(params, marketingrepo.CreateCampaignAttachmentParams{
			CampaignID: campaignID,
			FileName:   file.FileName,
			FilePath:   file.FilePath,
			FileType:   file.FileType,
			FileSize:   file.FileSize,
			UploadedBy: principal.UserID,
		})
	}

	result, err := h.service.AddAttachments(r.Context(), params)
	if err != nil {
		for _, uploaded := range uploadedFiles {
			_ = os.Remove(filepath.Join(h.uploadsDir, filepath.FromSlash(uploaded.FilePath)))
		}
		h.writeError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "upload_attachments", "marketing", "campaign", campaignID, nil, nil)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CampaignsHandler) listAttachments(w http.ResponseWriter, r *http.Request) {
	campaignID, ok := validateCampaignUUIDParam(w, "campaignID", chi.URLParam(r, "campaignID"))
	if !ok {
		return
	}
	items, err := h.service.ListAttachments(r.Context(), campaignID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *CampaignsHandler) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	campaignID, ok := validateCampaignUUIDParam(w, "campaignID", chi.URLParam(r, "campaignID"))
	if !ok {
		return
	}
	attachmentID, ok := validateCampaignUUIDParam(w, "attachmentID", chi.URLParam(r, "attachmentID"))
	if !ok {
		return
	}
	item, err := h.service.DeleteAttachment(r.Context(), campaignID, attachmentID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	_ = os.Remove(filepath.Join(h.uploadsDir, filepath.FromSlash(item.FilePath)))
	platformmiddleware.AuditLog(r.Context(), "delete", "marketing", "campaign_attachment", attachmentID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Campaign attachment deleted successfully"}, nil)
}

func (h *CampaignsHandler) listColumns(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListColumns(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *CampaignsHandler) createColumn(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMarketingAdmin(w, r); !ok {
		return
	}

	var input marketingdto.CreateCampaignColumnRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	item, err := h.service.CreateColumn(r.Context(), input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "marketing", "campaign_column", item.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, item, nil)
}

func (h *CampaignsHandler) updateColumn(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMarketingAdmin(w, r); !ok {
		return
	}

	var input marketingdto.UpdateCampaignColumnRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	columnID := chi.URLParam(r, "columnID")
	item, err := h.service.UpdateColumn(r.Context(), columnID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "marketing", "campaign_column", columnID, nil, input)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *CampaignsHandler) deleteColumn(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMarketingAdmin(w, r); !ok {
		return
	}

	columnID := chi.URLParam(r, "columnID")
	if err := h.service.DeleteColumn(r.Context(), columnID); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "marketing", "campaign_column", columnID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Campaign column deleted successfully"}, nil)
}

func (h *CampaignsHandler) reorderColumns(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMarketingAdmin(w, r); !ok {
		return
	}

	var input marketingdto.ReorderCampaignColumnsRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	if err := h.service.ReorderColumns(r.Context(), input); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "reorder", "marketing", "campaign_columns", "bulk", nil, input)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Campaign columns reordered successfully"}, nil)
}

func (h *CampaignsHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (marketingdto.ListCampaignsQuery, bool) {
	query := marketingdto.ListCampaignsQuery{
		Search:   r.URL.Query().Get("search"),
		Channel:  r.URL.Query().Get("channel"),
		Status:   r.URL.Query().Get("status"),
		PIC:      r.URL.Query().Get("pic"),
		DateFrom: r.URL.Query().Get("date_from"),
		DateTo:   r.URL.Query().Get("date_to"),
	}

	if value := strings.TrimSpace(r.URL.Query().Get("page")); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return marketingdto.ListCampaignsQuery{}, false
		}
		query.Page = page
	}

	if value := strings.TrimSpace(r.URL.Query().Get("per_page")); value != "" {
		perPage, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return marketingdto.ListCampaignsQuery{}, false
		}
		query.PerPage = perPage
	}

	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return marketingdto.ListCampaignsQuery{}, false
	}

	return query, true
}

func (h *CampaignsHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, marketingservice.ErrCampaignNotFound):
		response.WriteError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, marketingservice.ErrCampaignColumnNotFound):
		response.WriteError(w, http.StatusNotFound, "CAMPAIGN_COLUMN_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, marketingservice.ErrCampaignAttachmentNotFound):
		response.WriteError(w, http.StatusNotFound, "CAMPAIGN_ATTACHMENT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, marketingservice.ErrCampaignPICNotFound):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"pic_employee_id": "not found"})
	case errors.Is(err, marketingservice.ErrCampaignColumnInUse):
		response.WriteError(w, http.StatusConflict, "CAMPAIGN_COLUMN_IN_USE", err.Error(), nil)
	default:
		response.WriteInternalError(ctx, w, err, "An unexpected error occurred")
	}
}

type uploadedCampaignFile struct {
	FileName string
	FilePath string
	FileType string
	FileSize int64
}

var (
	errCampaignAttachmentValidation = errors.New("campaign attachment validation failed")
	errCampaignAttachmentStorage    = errors.New("campaign attachment storage failed")
)

func saveCampaignAttachments(baseUploadsDir string, files []*multipart.FileHeader) ([]uploadedCampaignFile, error) {
	dir := filepath.Join(baseUploadsDir, "campaigns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: %w", errCampaignAttachmentStorage, err)
	}

	items := make([]uploadedCampaignFile, 0, len(files))
	for _, file := range files {
		validation, err := uploads.ValidateMultipartFile(uploads.KindCampaignAttachment, file)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errCampaignAttachmentValidation, err)
		}
		contentType := validation.ContentType

		src, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errCampaignAttachmentStorage, err)
		}

		filename := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + sanitizeCampaignFilename(file.Filename)
		destinationPath := filepath.Join(dir, filename)
		dst, err := os.Create(destinationPath)
		if err != nil {
			_ = src.Close()
			return nil, fmt.Errorf("%w: %w", errCampaignAttachmentStorage, err)
		}

		if _, err := io.Copy(dst, src); err != nil {
			_ = dst.Close()
			_ = src.Close()
			_ = os.Remove(destinationPath)
			return nil, fmt.Errorf("%w: %w", errCampaignAttachmentStorage, err)
		}

		_ = dst.Close()
		_ = src.Close()

		items = append(items, uploadedCampaignFile{
			FileName: file.Filename,
			FilePath: filepath.ToSlash(filepath.Join("campaigns", filename)),
			FileType: contentType,
			FileSize: file.Size,
		})
	}

	return items, nil
}

func sanitizeCampaignFilename(value string) string {
	filename := strings.ToLower(strings.TrimSpace(value))
	filename = strings.ReplaceAll(filename, " ", "-")
	filename = strings.ReplaceAll(filename, "..", "")
	return filename
}

func validateCampaignUUIDParam(w http.ResponseWriter, field string, value string) (string, bool) {
	value = strings.TrimSpace(value)
	if _, err := uuid.Parse(value); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Path validation failed", map[string]string{field: "must be a valid UUID"})
		return "", false
	}
	return value, true
}
