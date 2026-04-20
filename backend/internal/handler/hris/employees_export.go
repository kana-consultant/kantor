package hris

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	exportreport "github.com/kana-consultant/kantor/backend/internal/export"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
)

func (h *EmployeesHandler) exportList(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	query.Page = 1
	query.PerPage = 10000

	items, _, _, _, err := h.service.ListEmployees(r.Context(), query)
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
		payload, err = renderEmployeesPDF(items, exportutil.ResolveGeneratedBy(r.Context(), h.users))
		contentType = "application/pdf"
		filename = exportutil.Filename("employees", "pdf")
	case "xlsx":
		payload, err = renderEmployeesXLSX(items)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = exportutil.Filename("employees", "xlsx")
	default:
		responseUnsupportedFormat(w)
		return
	}
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "hris", "employee", "filtered", nil, map[string]any{
		"format":     format,
		"count":      len(items),
		"department": query.Department,
		"status":     query.EmploymentStatus,
	})
	writeBinaryAttachment(w, contentType, filename, payload)
}

func (h *EmployeesHandler) exportDetail(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		responseUnauthorized(w)
		return
	}

	employeeID := chi.URLParam(r, "employeeID")
	item, err := h.service.GetEmployee(r.Context(), employeeID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	salaries, err := h.compensation.ListSalaries(r.Context(), employeeID, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	bonuses, err := h.compensation.ListBonuses(r.Context(), employeeID, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	payload, err := renderEmployeeDetailPDF(item, salaries, bonuses, exportutil.ResolveGeneratedBy(r.Context(), h.users))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "hris", "employee_profile", employeeID, nil, map[string]any{
		"format": "pdf",
	})
	writeBinaryAttachment(w, "application/pdf", exportutil.Filename("employee-profile", "pdf"), payload)
}

func renderEmployeesPDF(items []model.Employee, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Employees Report", "hris", generatedBy)
	report.AddSummary(map[string]string{
		"Employees":          strconv.Itoa(len(items)),
		"Active employees":   strconv.Itoa(countEmployeesByStatus(items, "active")),
		"Inactive employees": strconv.Itoa(countEmployeesByStatus(items, "inactive")),
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.FullName,
			item.Position,
			exportutil.OptionalString(item.Department, "-"),
			item.EmploymentStatus,
			exportutil.FormatDate(item.DateJoined),
			item.Email,
		})
	}
	report.AddTable([]string{"Name", "Role", "Department", "Status", "Date Joined", "Contact"}, rows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderEmployeesXLSX(items []model.Employee) ([]byte, error) {
	report := exportreport.NewExcelReport("Employees Report", "hris")
	sheet := report.AddSheet("Employees")
	if err := report.WriteHeader(sheet, 1, []string{"Name", "Role", "Department", "Status", "Date Joined", "Email", "Phone"}); err != nil {
		return nil, err
	}

	rows := make([][]exportreport.CellValue, 0, len(items))
	for _, item := range items {
		rows = append(rows, []exportreport.CellValue{
			exportreport.TextCell(item.FullName),
			exportreport.TextCell(item.Position),
			exportreport.TextCell(exportutil.OptionalString(item.Department, "-")),
			exportreport.TextCell(item.EmploymentStatus),
			exportreport.DateCell(item.DateJoined),
			exportreport.TextCell(item.Email),
			exportreport.TextCell(exportutil.OptionalString(item.Phone, "-")),
		})
	}
	if err := report.WriteRows(sheet, 2, rows); err != nil {
		return nil, err
	}

	if err := report.AddSummarySheet(map[string]string{
		"Employees":          strconv.Itoa(len(items)),
		"Active employees":   strconv.Itoa(countEmployeesByStatus(items, "active")),
		"Inactive employees": strconv.Itoa(countEmployeesByStatus(items, "inactive")),
	}); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderEmployeeDetailPDF(employee model.Employee, salaries []model.SalaryRecord, bonuses []model.BonusRecord, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Employee Profile Report", "hris", generatedBy)
	report.AddSummary(map[string]string{
		"Name":        employee.FullName,
		"Role":        employee.Position,
		"Department":  exportutil.OptionalString(employee.Department, "-"),
		"Status":      employee.EmploymentStatus,
		"Date joined": exportutil.FormatDate(employee.DateJoined),
		"Email":       employee.Email,
	})

	report.AddSection("Salary History")
	salaryRows := make([][]string, 0, len(salaries))
	for _, item := range salaries {
		salaryRows = append(salaryRows, []string{
			exportutil.FormatDate(item.EffectiveDate),
			exportutil.FormatIDR(item.BaseSalary),
			exportutil.FormatIDR(sumSalaryParts(item.Allowances)),
			exportutil.FormatIDR(sumSalaryParts(item.Deductions)),
			exportutil.FormatIDR(item.NetSalary),
		})
	}
	report.AddTable([]string{"Effective Date", "Base Salary", "Allowances", "Deductions", "Net Salary"}, salaryRows)

	report.AddSection("Bonus History")
	bonusRows := make([][]string, 0, len(bonuses))
	for _, item := range bonuses {
		bonusRows = append(bonusRows, []string{
			strconv.Itoa(item.PeriodMonth) + "/" + strconv.Itoa(item.PeriodYear),
			exportutil.FormatIDR(item.Amount),
			item.ApprovalStatus,
			item.Reason,
		})
	}
	report.AddTable([]string{"Period", "Amount", "Status", "Reason"}, bonusRows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func countEmployeesByStatus(items []model.Employee, status string) int {
	count := 0
	for _, item := range items {
		if strings.EqualFold(item.EmploymentStatus, status) {
			count++
		}
	}
	return count
}

func sumSalaryParts(values map[string]int64) int64 {
	var total int64
	for _, item := range values {
		total += item
	}
	return total
}
