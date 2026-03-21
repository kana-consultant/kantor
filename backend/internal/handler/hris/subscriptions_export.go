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

func (h *SubscriptionsHandler) export(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListSubscriptions(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	summary, err := h.service.Summary(r.Context())
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
		payload, err = renderSubscriptionsPDF(items, summary, exportutil.ResolveGeneratedBy(r.Context(), h.users))
		contentType = "application/pdf"
		filename = exportutil.Filename("subscriptions", "pdf")
	case "xlsx":
		payload, err = renderSubscriptionsXLSX(items, summary)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = exportutil.Filename("subscriptions", "xlsx")
	default:
		responseUnsupportedFormat(w)
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "hris", "subscription", "all", nil, map[string]any{
		"format": format,
		"count":  len(items),
	})
	writeBinaryAttachment(w, contentType, filename, payload)
}

func renderSubscriptionsPDF(items []model.Subscription, summary model.SubscriptionSummary, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Subscription Report", "hris", generatedBy)
	report.AddSummary(map[string]string{
		"Subscriptions":      strconv.Itoa(len(items)),
		"Active subscriptions": strconv.FormatInt(summary.ActiveCount, 10),
		"Total monthly cost": exportutil.FormatIDR(summary.TotalMonthlyCost),
		"Total yearly cost":  exportutil.FormatIDR(summary.TotalYearlyCost),
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.Name,
			item.Vendor,
			item.Category,
			item.BillingCycle,
			exportutil.FormatIDR(item.CostAmount),
			item.Status,
			exportutil.FormatDate(item.RenewalDate),
		})
	}
	report.AddTable([]string{"Name", "Vendor", "Category", "Cycle", "Cost", "Status", "Renewal"}, rows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderSubscriptionsXLSX(items []model.Subscription, summary model.SubscriptionSummary) ([]byte, error) {
	report := exportreport.NewExcelReport("Subscription Report", "hris")
	sheet := report.AddSheet("Subscriptions")
	if err := report.WriteHeader(sheet, 1, []string{"Name", "Vendor", "Category", "Billing Cycle", "Cost", "Status", "Renewal Date", "PIC"}); err != nil {
		return nil, err
	}

	rows := make([][]exportreport.CellValue, 0, len(items))
	for _, item := range items {
		rows = append(rows, []exportreport.CellValue{
			exportreport.TextCell(item.Name),
			exportreport.TextCell(item.Vendor),
			exportreport.TextCell(item.Category),
			exportreport.TextCell(item.BillingCycle),
			exportreport.CurrencyCell(item.CostAmount),
			exportreport.TextCell(item.Status),
			exportreport.DateCell(item.RenewalDate),
			exportreport.TextCell(exportutil.OptionalString(item.PICEmployeeName, "-")),
		})
	}
	if err := report.WriteRows(sheet, 2, rows); err != nil {
		return nil, err
	}

	if err := report.AddSummarySheet(map[string]string{
		"Subscriptions":       strconv.Itoa(len(items)),
		"Active subscriptions": strconv.FormatInt(summary.ActiveCount, 10),
		"Total monthly cost":  exportutil.FormatIDR(summary.TotalMonthlyCost),
		"Total yearly cost":   exportutil.FormatIDR(summary.TotalYearlyCost),
	}); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

