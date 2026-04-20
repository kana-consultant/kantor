package hris

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"

	exportreport "github.com/kana-consultant/kantor/backend/internal/export"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
)

func (h *ReimbursementsHandler) export(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		responseUnauthorized(w)
		return
	}

	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	query.Page = 1
	query.PerPage = 10000

	items, _, _, _, err := h.service.List(r.Context(), query, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	summary, err := h.service.Summary(r.Context(), query.Month, query.Year, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "pdf"
	}

	var (
		payload     []byte
		contentType string
		filename    string
	)

	switch format {
	case "pdf":
		payload, err = renderReimbursementsPDF(items, summary, exportutil.ResolveGeneratedBy(r.Context(), h.users))
		contentType = "application/pdf"
		filename = exportutil.Filename("reimbursements", "pdf")
	case "xlsx":
		payload, err = renderReimbursementsXLSX(items, summary)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = exportutil.Filename("reimbursements", "xlsx")
	default:
		responseUnsupportedFormat(w)
		return
	}
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "hris", "reimbursement", "filtered", nil, map[string]any{
		"format": format,
		"count":  len(items),
		"month":  query.Month,
		"year":   query.Year,
		"status": query.Status,
	})

	writeBinaryAttachment(w, contentType, filename, payload)
}

func renderReimbursementsPDF(items []model.Reimbursement, summary model.ReimbursementSummary, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Reimbursement Report", "hris", generatedBy)
	report.AddSummary(map[string]string{
		"Requests":         strconv.Itoa(len(items)),
		"Grand total":      exportutil.FormatIDR(totalReimbursements(items)),
		"Approved month":   exportutil.FormatIDR(summary.ApprovedAmountMonth),
		"Awaiting review":  strconv.Itoa(summary.CountsByStatus["submitted"]),
		"Approved records": strconv.Itoa(summary.CountsByStatus["approved"]),
		"Paid records":     strconv.Itoa(summary.CountsByStatus["paid"]),
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.EmployeeName,
			item.Title,
			item.Category,
			exportutil.FormatIDR(item.Amount),
			item.Status,
			exportutil.FormatDate(item.TransactionDate),
		})
	}
	report.AddTable([]string{"Employee", "Title", "Category", "Amount", "Status", "Transaction Date"}, rows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderReimbursementsXLSX(items []model.Reimbursement, summary model.ReimbursementSummary) ([]byte, error) {
	report := exportreport.NewExcelReport("Reimbursement Report", "hris")
	sheet := report.AddSheet("Reimbursements")
	if err := report.WriteHeader(sheet, 1, []string{"Employee", "Title", "Category", "Amount", "Status", "Transaction Date"}); err != nil {
		return nil, err
	}

	rows := make([][]exportreport.CellValue, 0, len(items))
	for _, item := range items {
		rows = append(rows, []exportreport.CellValue{
			exportreport.TextCell(item.EmployeeName),
			exportreport.TextCell(item.Title),
			exportreport.TextCell(item.Category),
			exportreport.CurrencyCell(item.Amount),
			exportreport.TextCell(item.Status),
			exportreport.DateCell(item.TransactionDate),
		})
	}
	if err := report.WriteRows(sheet, 2, rows); err != nil {
		return nil, err
	}

	if err := report.AddSummarySheet(map[string]string{
		"Requests":        strconv.Itoa(len(items)),
		"Grand total":     exportutil.FormatIDR(totalReimbursements(items)),
		"Approved month":  exportutil.FormatIDR(summary.ApprovedAmountMonth),
		"Submitted count": strconv.Itoa(summary.CountsByStatus["submitted"]),
		"Approved count":  strconv.Itoa(summary.CountsByStatus["approved"]),
		"Paid count":      strconv.Itoa(summary.CountsByStatus["paid"]),
	}); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func totalReimbursements(items []model.Reimbursement) int64 {
	var total int64
	for _, item := range items {
		total += item.Amount
	}
	return total
}

