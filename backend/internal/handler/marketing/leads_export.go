package marketing

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

func (h *LeadsHandler) export(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	query.Page = 1
	query.PerPage = 10000

	items, _, _, _, err := h.service.ListLeads(r.Context(), query)
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
		payload, err = renderLeadsPDF(items, exportutil.ResolveGeneratedBy(r.Context(), h.users))
		contentType = "application/pdf"
		filename = exportutil.Filename("leads", "pdf")
	case "xlsx":
		payload, err = renderLeadsXLSX(items)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = exportutil.Filename("leads", "xlsx")
	default:
		responseUnsupportedFormat(w, false)
		return
	}
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "marketing", "lead", "filtered", nil, map[string]any{
		"format": format,
		"count":  len(items),
	})
	writeBinaryAttachment(w, contentType, filename, payload)
}

func renderLeadsPDF(items []model.Lead, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Leads Report", "marketing", generatedBy)
	report.AddSummary(map[string]string{
		"Leads":          strconv.Itoa(len(items)),
		"Pipeline value": exportutil.FormatIDR(totalLeadValue(items)),
		"Won":            strconv.Itoa(countLeadsByStatus(items, "won")),
		"Conversion":     leadConversionLabel(items),
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.Name,
			item.PipelineStatus,
			item.SourceChannel,
			exportutil.OptionalString(item.AssignedToName, "Unassigned"),
			exportutil.FormatIDR(item.EstimatedValue),
			exportutil.OptionalString(item.CampaignName, "-"),
		})
	}
	report.AddTable([]string{"Lead", "Status", "Source", "Assigned", "Estimated Value", "Campaign"}, rows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderLeadsXLSX(items []model.Lead) ([]byte, error) {
	report := exportreport.NewExcelReport("Leads Report", "marketing")
	leadsSheet := report.AddSheet("Leads")
	if err := report.WriteHeader(leadsSheet, 1, []string{"Lead", "Status", "Source", "Assigned", "Estimated Value", "Campaign", "Created At"}); err != nil {
		return nil, err
	}

	rows := make([][]exportreport.CellValue, 0, len(items))
	for _, item := range items {
		rows = append(rows, []exportreport.CellValue{
			exportreport.TextCell(item.Name),
			exportreport.TextCell(item.PipelineStatus),
			exportreport.TextCell(item.SourceChannel),
			exportreport.TextCell(exportutil.OptionalString(item.AssignedToName, "Unassigned")),
			exportreport.CurrencyCell(item.EstimatedValue),
			exportreport.TextCell(exportutil.OptionalString(item.CampaignName, "-")),
			exportreport.DateCell(item.CreatedAt),
		})
	}
	if err := report.WriteRows(leadsSheet, 2, rows); err != nil {
		return nil, err
	}

	if err := report.AddSummarySheet(map[string]string{
		"Leads":          strconv.Itoa(len(items)),
		"Pipeline value": exportutil.FormatIDR(totalLeadValue(items)),
		"Won":            strconv.Itoa(countLeadsByStatus(items, "won")),
		"Conversion":     leadConversionLabel(items),
	}); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func totalLeadValue(items []model.Lead) int64 {
	var total int64
	for _, item := range items {
		total += item.EstimatedValue
	}
	return total
}

func countLeadsByStatus(items []model.Lead, status string) int {
	count := 0
	for _, item := range items {
		if strings.EqualFold(item.PipelineStatus, status) {
			count++
		}
	}
	return count
}

func leadConversionLabel(items []model.Lead) string {
	if len(items) == 0 {
		return "0.00%"
	}
	rate := (float64(countLeadsByStatus(items, "won")) / float64(len(items))) * 100
	return strconv.FormatFloat(rate, 'f', 2, 64) + "%"
}
