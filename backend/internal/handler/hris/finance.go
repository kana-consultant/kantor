package hris

import (
	"bytes"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	exportreport "github.com/kana-consultant/kantor/backend/internal/export"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

type FinanceHandler struct {
	service   *hrisservice.FinanceService
	users     exportutil.UserLookup
	validator *validator.Validate
}

func NewFinanceHandler(service *hrisservice.FinanceService, users exportutil.UserLookup) *FinanceHandler {
	return &FinanceHandler{
		service:   service,
		users:     users,
		validator: newValidator(),
	}
}

func (h *FinanceHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/categories", h.listCategories)
	router.With(platformmiddleware.RequirePermission("hris:finance:approve")).Post("/categories", h.createCategory)
	router.With(platformmiddleware.RequirePermission("hris:finance:approve")).Put("/categories/{categoryID}", h.updateCategory)
	router.With(platformmiddleware.RequirePermission("hris:finance:approve")).Delete("/categories/{categoryID}", h.deleteCategory)

	router.With(platformmiddleware.RequirePermission("hris:finance:create")).Post("/records", h.createRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/records", h.listRecords)
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/records/{recordID}", h.getRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:edit")).Put("/records/{recordID}", h.updateRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:delete")).Delete("/records/{recordID}", h.deleteRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:create")).Patch("/records/{recordID}/submit", h.submitRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:approve")).Patch("/records/{recordID}/review", h.reviewRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/summary", h.summary)
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/export", h.exportRecords)
}

func (h *FinanceHandler) createCategory(w http.ResponseWriter, r *http.Request) {
	if !requireFinanceAdmin(w, r) {
		return
	}
	var input hrisdto.CreateFinanceCategoryRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	result, err := h.service.CreateCategory(r.Context(), input)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "finance_category", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *FinanceHandler) listCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListCategories(r.Context(), r.URL.Query().Get("type"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *FinanceHandler) updateCategory(w http.ResponseWriter, r *http.Request) {
	if !requireFinanceAdmin(w, r) {
		return
	}
	var input hrisdto.UpdateFinanceCategoryRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	categoryID := chi.URLParam(r, "categoryID")
	result, err := h.service.UpdateCategory(r.Context(), categoryID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "hris", "finance_category", categoryID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	if !requireFinanceAdmin(w, r) {
		return
	}
	categoryID := chi.URLParam(r, "categoryID")
	if err := h.service.DeleteCategory(r.Context(), categoryID); err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "finance_category", categoryID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Finance category deleted successfully"}, nil)
}

func requireFinanceAdmin(w http.ResponseWriter, r *http.Request) bool {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return false
	}
	if principal.IsSuperAdmin {
		return true
	}
	if principal.ModuleRoles != nil {
		if role, exists := principal.ModuleRoles["hris"]; exists && role.RoleSlug == "admin" {
			return true
		}
	}
	response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Finance categories can only be managed by HRIS admin", nil)
	return false
}

func (h *FinanceHandler) createRecord(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.CreateFinanceRecordRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	result, err := h.service.CreateRecord(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "finance_record", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *FinanceHandler) listRecords(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	principal, principalOK := platformmiddleware.PrincipalFromContext(r.Context())
	if !principalOK {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	items, total, page, perPage, err := h.service.ListRecords(r.Context(), query, principal.UserID, principal.Cached)
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

func (h *FinanceHandler) getRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	result, err := h.service.GetRecord(r.Context(), chi.URLParam(r, "recordID"), principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) updateRecord(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.UpdateFinanceRecordRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	recordID := chi.URLParam(r, "recordID")
	result, err := h.service.UpdateRecord(r.Context(), recordID, input, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "hris", "finance_record", recordID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) deleteRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	recordID := chi.URLParam(r, "recordID")
	if err := h.service.DeleteRecord(r.Context(), recordID, principal.UserID, principal.Cached); err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "finance_record", recordID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Finance record deleted successfully"}, nil)
}

func (h *FinanceHandler) submitRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	recordID := chi.URLParam(r, "recordID")
	result, err := h.service.SubmitRecord(r.Context(), recordID, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "submit", "hris", "finance_record", recordID, nil, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) reviewRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	var input hrisdto.ReviewFinanceRecordRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	recordID := chi.URLParam(r, "recordID")
	result, err := h.service.ReviewRecord(r.Context(), recordID, input.Decision, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "review", "hris", "finance_record", recordID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) summary(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("year")))
	if err != nil || year == 0 {
		year = time.Now().Year()
	}
	result, err := h.service.Summary(r.Context(), year)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) exportRecords(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	query.Page = 1
	query.PerPage = 10000

	items, _, _, _, err := h.service.ListRecords(r.Context(), query, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}

	selectedYear := query.Year
	if selectedYear == 0 {
		selectedYear = time.Now().Year()
	}
	summary, err := h.service.Summary(r.Context(), selectedYear)
	if err != nil {
		h.writeError(w, err)
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "csv"
	}

	var (
		contentType string
		filename    string
		payload     []byte
	)

	switch format {
	case "csv":
		payload, err = renderFinanceCSV(items)
		contentType = "text/csv"
		filename = exportutil.Filename("finance-records", "csv")
	case "pdf":
		payload, err = renderFinancePDF(items, summary, exportutil.ResolveGeneratedBy(r.Context(), h.users))
		contentType = "application/pdf"
		filename = exportutil.Filename("finance-records", "pdf")
	case "xlsx":
		payload, err = renderFinanceXLSX(items, summary)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = exportutil.Filename("finance-records", "xlsx")
	default:
		response.WriteError(w, http.StatusBadRequest, "UNSUPPORTED_EXPORT_FORMAT", "Export format is not supported", map[string]string{"format": "must be csv, pdf, or xlsx"})
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "hris", "finance_record", "filtered", nil, map[string]any{
		"format": format,
		"count":  len(items),
		"year":   query.Year,
		"month":  query.Month,
		"type":   query.Type,
		"status": query.Status,
	})

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (h *FinanceHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (hrisdto.ListFinanceRecordsQuery, bool) {
	query := hrisdto.ListFinanceRecordsQuery{
		Type:       r.URL.Query().Get("type"),
		CategoryID: r.URL.Query().Get("category"),
		Status:     r.URL.Query().Get("status"),
	}
	if value := strings.TrimSpace(r.URL.Query().Get("page")); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return hrisdto.ListFinanceRecordsQuery{}, false
		}
		query.Page = page
	}
	if value := strings.TrimSpace(r.URL.Query().Get("per_page")); value != "" {
		perPage, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return hrisdto.ListFinanceRecordsQuery{}, false
		}
		query.PerPage = perPage
	}
	if value := strings.TrimSpace(r.URL.Query().Get("month")); value != "" {
		month, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"month": "must be a number"})
			return hrisdto.ListFinanceRecordsQuery{}, false
		}
		query.Month = month
	}
	if value := strings.TrimSpace(r.URL.Query().Get("year")); value != "" {
		year, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"year": "must be a number"})
			return hrisdto.ListFinanceRecordsQuery{}, false
		}
		query.Year = year
	}
	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return hrisdto.ListFinanceRecordsQuery{}, false
	}
	return query, true
}

func (h *FinanceHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrFinanceCategoryNotFound):
		response.WriteError(w, http.StatusNotFound, "FINANCE_CATEGORY_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrFinanceRecordNotFound):
		response.WriteError(w, http.StatusNotFound, "FINANCE_RECORD_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrFinanceCategoryExists):
		response.WriteError(w, http.StatusConflict, "FINANCE_CATEGORY_EXISTS", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrFinanceForbidden):
		response.WriteError(w, http.StatusForbidden, "FINANCE_FORBIDDEN", err.Error(), nil)
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}

func renderFinanceCSV(items []model.FinanceRecord) ([]byte, error) {
	builder := &strings.Builder{}
	writer := csv.NewWriter(builder)
	if err := writer.Write([]string{"id", "category", "type", "amount", "description", "record_date", "status"}); err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := writer.Write([]string{
			item.ID,
			item.CategoryName,
			item.Type,
			strconv.FormatInt(item.Amount, 10),
			item.Description,
			item.RecordDate.Format("2006-01-02"),
			item.ApprovalStatus,
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}

func renderFinancePDF(items []model.FinanceRecord, summary model.FinanceSummary, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Finance Records Report", "hris", generatedBy)
	report.AddParagraph("Approved and in-flight finance records exported from the HRIS finance workspace.")
	report.AddSummary(map[string]string{
		"Filtered records": strconv.Itoa(len(items)),
		"Total income":     exportutil.FormatIDR(totalFinanceByType(items, "income")),
		"Total outcome":    exportutil.FormatIDR(totalFinanceByType(items, "outcome")),
		"Net":              exportutil.FormatIDR(totalFinanceByType(items, "income") - totalFinanceByType(items, "outcome")),
		"Yearly total":     exportutil.FormatIDR(summary.TotalIncome - summary.TotalOutcome),
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.CategoryName,
			strings.ToUpper(item.Type),
			exportutil.FormatIDR(item.Amount),
			item.ApprovalStatus,
			exportutil.FormatDate(item.RecordDate),
			item.Description,
		})
	}
	report.AddTable([]string{"Category", "Type", "Amount", "Status", "Record Date", "Description"}, rows)

	report.AddSection("Monthly Summary")
	monthlyRows := make([][]string, 0, len(summary.Monthly))
	for _, month := range summary.Monthly {
		if month.Income == 0 && month.Outcome == 0 {
			continue
		}
		monthlyRows = append(monthlyRows, []string{
			time.Month(month.Month).String(),
			exportutil.FormatIDR(month.Income),
			exportutil.FormatIDR(month.Outcome),
			exportutil.FormatIDR(month.Income - month.Outcome),
		})
	}
	report.AddTable([]string{"Month", "Income", "Outcome", "Net"}, monthlyRows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderFinanceXLSX(items []model.FinanceRecord, summary model.FinanceSummary) ([]byte, error) {
	report := exportreport.NewExcelReport("Finance Records Report", "hris")
	recordsSheet := report.AddSheet("Records")
	if err := report.WriteHeader(recordsSheet, 1, []string{"Category", "Type", "Amount", "Status", "Record Date", "Description"}); err != nil {
		return nil, err
	}

	rows := make([][]exportreport.CellValue, 0, len(items))
	for _, item := range items {
		rows = append(rows, []exportreport.CellValue{
			exportreport.TextCell(item.CategoryName),
			exportreport.TextCell(strings.ToUpper(item.Type)),
			exportreport.CurrencyCell(item.Amount),
			exportreport.TextCell(item.ApprovalStatus),
			exportreport.DateCell(item.RecordDate),
			exportreport.TextCell(item.Description),
		})
	}
	if err := report.WriteRows(recordsSheet, 2, rows); err != nil {
		return nil, err
	}

	summarySheet := report.AddSheet("Summary")
	if err := report.WriteHeader(summarySheet, 1, []string{"Month", "Income", "Outcome", "Net"}); err != nil {
		return nil, err
	}
	summaryRows := make([][]exportreport.CellValue, 0, len(summary.Monthly))
	for _, month := range summary.Monthly {
		summaryRows = append(summaryRows, []exportreport.CellValue{
			exportreport.TextCell(time.Month(month.Month).String()),
			exportreport.CurrencyCell(month.Income),
			exportreport.CurrencyCell(month.Outcome),
			exportreport.CurrencyCell(month.Income - month.Outcome),
		})
	}
	if err := report.WriteRows(summarySheet, 2, summaryRows); err != nil {
		return nil, err
	}

	if err := report.AddSummarySheet(map[string]string{
		"Filtered records": strconv.Itoa(len(items)),
		"Total income":     exportutil.FormatIDR(totalFinanceByType(items, "income")),
		"Total outcome":    exportutil.FormatIDR(totalFinanceByType(items, "outcome")),
		"Yearly total":     exportutil.FormatIDR(summary.TotalIncome - summary.TotalOutcome),
	}); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func totalFinanceByType(items []model.FinanceRecord, recordType string) int64 {
	var total int64
	for _, item := range items {
		if strings.EqualFold(item.Type, recordType) {
			total += item.Amount
		}
	}
	return total
}
