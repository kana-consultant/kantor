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

func (h *CampaignsHandler) export(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	query.Page = 1
	query.PerPage = 10000

	items, _, _, _, err := h.service.ListCampaigns(r.Context(), query)
	if err != nil {
		h.writeError(w, err)
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
		payload, err = renderCampaignsPDF(items, exportutil.ResolveGeneratedBy(r.Context(), h.users))
		contentType = "application/pdf"
		filename = exportutil.Filename("campaigns", "pdf")
	case "xlsx":
		payload, err = renderCampaignsXLSX(items)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = exportutil.Filename("campaigns", "xlsx")
	default:
		responseUnsupportedFormat(w, false)
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "marketing", "campaign", "filtered", nil, map[string]any{
		"format": format,
		"count":  len(items),
	})
	writeBinaryAttachment(w, contentType, filename, payload)
}

func renderCampaignsPDF(items []model.Campaign, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Campaigns Report", "marketing", generatedBy)
	report.AddSummary(map[string]string{
		"Campaigns":    strconv.Itoa(len(items)),
		"Total budget": exportutil.FormatIDR(totalCampaignBudget(items)),
		"Live":         strconv.Itoa(countCampaignsByStatus(items, "live")),
		"Planning":     strconv.Itoa(countCampaignsByStatus(items, "planning")),
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.Name,
			item.Channel,
			item.Status,
			exportutil.FormatIDR(item.BudgetAmount),
			exportutil.OptionalString(item.PICEmployeeName, "Unassigned"),
			exportutil.FormatDate(item.StartDate) + " - " + exportutil.FormatDate(item.EndDate),
		})
	}
	report.AddTable([]string{"Campaign", "Channel", "Status", "Budget", "PIC", "Timeline"}, rows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderCampaignsXLSX(items []model.Campaign) ([]byte, error) {
	report := exportreport.NewExcelReport("Campaigns Report", "marketing")
	sheet := report.AddSheet("Campaigns")
	if err := report.WriteHeader(sheet, 1, []string{"Campaign", "Channel", "Status", "Budget", "PIC", "Start Date", "End Date"}); err != nil {
		return nil, err
	}

	rows := make([][]exportreport.CellValue, 0, len(items))
	for _, item := range items {
		rows = append(rows, []exportreport.CellValue{
			exportreport.TextCell(item.Name),
			exportreport.TextCell(item.Channel),
			exportreport.TextCell(item.Status),
			exportreport.CurrencyCell(item.BudgetAmount),
			exportreport.TextCell(exportutil.OptionalString(item.PICEmployeeName, "Unassigned")),
			exportreport.DateCell(item.StartDate),
			exportreport.DateCell(item.EndDate),
		})
	}
	if err := report.WriteRows(sheet, 2, rows); err != nil {
		return nil, err
	}
	if err := report.AddSummarySheet(map[string]string{
		"Campaigns":    strconv.Itoa(len(items)),
		"Total budget": exportutil.FormatIDR(totalCampaignBudget(items)),
		"Live":         strconv.Itoa(countCampaignsByStatus(items, "live")),
		"Planning":     strconv.Itoa(countCampaignsByStatus(items, "planning")),
	}); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func totalCampaignBudget(items []model.Campaign) int64 {
	var total int64
	for _, item := range items {
		total += item.BudgetAmount
	}
	return total
}

func countCampaignsByStatus(items []model.Campaign, status string) int {
	count := 0
	for _, item := range items {
		if strings.EqualFold(item.Status, status) {
			count++
		}
	}
	return count
}

