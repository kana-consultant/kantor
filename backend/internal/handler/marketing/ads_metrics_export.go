package marketing

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	exportreport "github.com/kana-consultant/kantor/backend/internal/export"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
)

func (h *AdsMetricsHandler) export(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	query.Page = 1
	query.PerPage = 10000

	items, _, _, _, err := h.service.ListMetrics(r.Context(), query)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "csv"
	}

	var (
		payload     []byte
		contentType string
		filename    string
	)
	switch format {
	case "csv":
		payload, err = renderAdsMetricsCSV(items)
		contentType = "text/csv"
		filename = exportutil.Filename("ads-metrics", "csv")
	case "pdf":
		payload, err = renderAdsMetricsPDF(items, exportutil.ResolveGeneratedBy(r.Context(), h.users))
		contentType = "application/pdf"
		filename = exportutil.Filename("ads-metrics", "pdf")
	case "xlsx":
		payload, err = renderAdsMetricsXLSX(items)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = exportutil.Filename("ads-metrics", "xlsx")
	default:
		responseUnsupportedFormat(w, true)
		return
	}
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "marketing", "ads_metric", "filtered", nil, map[string]any{
		"format":      format,
		"count":       len(items),
		"campaign_id": query.CampaignID,
		"platform":    query.Platform,
		"date_from":   query.DateFrom,
		"date_to":     query.DateTo,
	})
	writeBinaryAttachment(w, contentType, filename, payload)
}

func renderAdsMetricsCSV(items []model.AdsMetric) ([]byte, error) {
	builder := &strings.Builder{}
	writer := csv.NewWriter(builder)
	if err := writer.Write([]string{
		"id", "campaign_name", "platform", "period_start", "period_end",
		"amount_spent", "impressions", "clicks", "conversions", "revenue",
		"cpr", "roas", "ctr", "cpc", "cpm", "notes",
	}); err != nil {
		return nil, err
	}

	for _, item := range items {
		if err := writer.Write([]string{
			item.ID,
			exportutil.OptionalString(item.CampaignName, ""),
			item.Platform,
			item.PeriodStart.Format("2006-01-02"),
			item.PeriodEnd.Format("2006-01-02"),
			strconv.FormatInt(item.AmountSpent, 10),
			strconv.FormatInt(item.Impressions, 10),
			strconv.FormatInt(item.Clicks, 10),
			strconv.FormatInt(item.Conversions, 10),
			strconv.FormatInt(item.Revenue, 10),
			formatMetricFloat(item.CPR),
			formatMetricFloat(item.ROAS),
			formatMetricFloat(item.CTR),
			formatMetricFloat(item.CPC),
			formatMetricFloat(item.CPM),
			exportutil.OptionalString(item.Notes, ""),
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

func renderAdsMetricsPDF(items []model.AdsMetric, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Ads Metrics Report", "marketing", generatedBy)
	report.AddSummary(map[string]string{
		"Records":       strconv.Itoa(len(items)),
		"Total spent":   exportutil.FormatIDR(totalAdsSpent(items)),
		"Total revenue": exportutil.FormatIDR(totalAdsRevenue(items)),
		"Overall ROAS":  overallROASLabel(items),
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			exportutil.OptionalString(item.CampaignName, "Unknown"),
			item.Platform,
			exportutil.FormatIDR(item.AmountSpent),
			exportutil.FormatIDR(item.Revenue),
			formatMetricFloat(item.ROAS),
			formatMetricFloat(item.CTR),
		})
	}
	report.AddTable([]string{"Campaign", "Platform", "Spent", "Revenue", "ROAS", "CTR"}, rows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderAdsMetricsXLSX(items []model.AdsMetric) ([]byte, error) {
	report := exportreport.NewExcelReport("Ads Metrics Report", "marketing")
	dataSheet := report.AddSheet("Data")
	if err := report.WriteHeader(dataSheet, 1, []string{"Campaign", "Platform", "Period Start", "Period End", "Spent", "Revenue", "ROAS", "CTR"}); err != nil {
		return nil, err
	}
	rows := make([][]exportreport.CellValue, 0, len(items))
	for _, item := range items {
		rows = append(rows, []exportreport.CellValue{
			exportreport.TextCell(exportutil.OptionalString(item.CampaignName, "Unknown")),
			exportreport.TextCell(item.Platform),
			exportreport.DateCell(item.PeriodStart),
			exportreport.DateCell(item.PeriodEnd),
			exportreport.CurrencyCell(item.AmountSpent),
			exportreport.CurrencyCell(item.Revenue),
			exportreport.TextCell(formatMetricFloat(item.ROAS)),
			exportreport.TextCell(formatMetricFloat(item.CTR)),
		})
	}
	if err := report.WriteRows(dataSheet, 2, rows); err != nil {
		return nil, err
	}
	if err := report.AddSummarySheet(map[string]string{
		"Records":       strconv.Itoa(len(items)),
		"Total spent":   exportutil.FormatIDR(totalAdsSpent(items)),
		"Total revenue": exportutil.FormatIDR(totalAdsRevenue(items)),
		"Overall ROAS":  overallROASLabel(items),
	}); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func totalAdsSpent(items []model.AdsMetric) int64 {
	var total int64
	for _, item := range items {
		total += item.AmountSpent
	}
	return total
}

func totalAdsRevenue(items []model.AdsMetric) int64 {
	var total int64
	for _, item := range items {
		total += item.Revenue
	}
	return total
}

func overallROASLabel(items []model.AdsMetric) string {
	spent := totalAdsSpent(items)
	if spent == 0 {
		return "-"
	}
	return formatMetricFloat(pointerFloat(float64(totalAdsRevenue(items)) / float64(spent)))
}

func pointerFloat(value float64) *float64 {
	return &value
}

func formatMetricFloat(value *float64) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

