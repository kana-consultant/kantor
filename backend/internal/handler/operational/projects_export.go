package operational

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
	"github.com/kana-consultant/kantor/backend/internal/response"
)

func (h *ProjectsHandler) exportList(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	query.Page = 1
	query.PerPage = 10000

	items, _, _, _, err := h.service.ListProjects(r.Context(), query)
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
		payload, err = renderProjectsPDF(items, exportutil.ResolveGeneratedBy(r.Context(), h.users))
		contentType = "application/pdf"
		filename = exportutil.Filename("projects", "pdf")
	case "xlsx":
		payload, err = renderProjectsXLSX(items)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = exportutil.Filename("projects", "xlsx")
	default:
		response.WriteError(w, http.StatusBadRequest, "UNSUPPORTED_EXPORT_FORMAT", "Export format is not supported", map[string]string{"format": "must be pdf or xlsx"})
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "operational", "project", "filtered", nil, map[string]any{
		"format":   format,
		"count":    len(items),
		"status":   query.Status,
		"priority": query.Priority,
		"search":   query.Search,
	})
	writeBinaryAttachment(w, contentType, filename, payload)
}

func (h *ProjectsHandler) exportDetail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	detail, err := h.service.GetProject(r.Context(), projectID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	columns, err := h.kanban.ListColumns(r.Context(), projectID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	tasks, err := h.kanban.ListTasks(r.Context(), projectID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	payload, err := renderProjectDetailPDF(detail.Project, detail.Members, columns, tasks, exportutil.ResolveGeneratedBy(r.Context(), h.users))
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "export", "operational", "project_detail", projectID, nil, map[string]any{
		"format": "pdf",
	})
	writeBinaryAttachment(w, "application/pdf", exportutil.Filename("project-report", "pdf"), payload)
}

func renderProjectsPDF(items []model.Project, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Projects Report", "operational", generatedBy)
	report.AddSummary(map[string]string{
		"Projects":         strconv.Itoa(len(items)),
		"Active projects":  strconv.Itoa(countProjectsByStatus(items, "active")),
		"Completed":        strconv.Itoa(countProjectsByStatus(items, "completed")),
		"High priority":    strconv.Itoa(countProjectsByPriority(items, "high")),
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		deadline := "-"
		if item.Deadline != nil {
			deadline = exportutil.FormatDate(*item.Deadline)
		}
		rows = append(rows, []string{
			item.Name,
			item.Status,
			item.Priority,
			deadline,
			strconv.Itoa(item.MemberCount),
			item.AutoAssignMode,
		})
	}
	report.AddTable([]string{"Project", "Status", "Priority", "Deadline", "Members", "Auto Assign"}, rows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderProjectsXLSX(items []model.Project) ([]byte, error) {
	report := exportreport.NewExcelReport("Projects Report", "operational")
	sheet := report.AddSheet("Projects")
	if err := report.WriteHeader(sheet, 1, []string{"Project", "Status", "Priority", "Deadline", "Members", "Auto Assign"}); err != nil {
		return nil, err
	}

	rows := make([][]exportreport.CellValue, 0, len(items))
	for _, item := range items {
		deadline := exportreport.TextCell("-")
		if item.Deadline != nil {
			deadline = exportreport.DateCell(*item.Deadline)
		}
		rows = append(rows, []exportreport.CellValue{
			exportreport.TextCell(item.Name),
			exportreport.TextCell(item.Status),
			exportreport.TextCell(item.Priority),
			deadline,
			exportreport.NumberCell(item.MemberCount),
			exportreport.TextCell(item.AutoAssignMode),
		})
	}
	if err := report.WriteRows(sheet, 2, rows); err != nil {
		return nil, err
	}

	if err := report.AddSummarySheet(map[string]string{
		"Projects":        strconv.Itoa(len(items)),
		"Active projects": strconv.Itoa(countProjectsByStatus(items, "active")),
		"Completed":       strconv.Itoa(countProjectsByStatus(items, "completed")),
		"High priority":   strconv.Itoa(countProjectsByPriority(items, "high")),
	}); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderProjectDetailPDF(project model.Project, members []model.ProjectMember, columns []model.KanbanColumn, tasks []model.KanbanTask, generatedBy string) ([]byte, error) {
	report := exportreport.NewPDFReport("Project Detail Report", "operational", generatedBy)
	deadline := "-"
	if project.Deadline != nil {
		deadline = exportutil.FormatDate(*project.Deadline)
	}
	report.AddSummary(map[string]string{
		"Project":     project.Name,
		"Status":      project.Status,
		"Priority":    project.Priority,
		"Deadline":    deadline,
		"Members":     strconv.Itoa(len(members)),
		"Tasks":       strconv.Itoa(len(tasks)),
	})

	memberRows := make([][]string, 0, len(members))
	for _, member := range members {
		memberRows = append(memberRows, []string{
			member.FullName,
			member.UserEmail,
			member.RoleInProject,
			exportutil.FormatDate(member.AssignedAt),
		})
	}
	report.AddSection("Project Members")
	report.AddTable([]string{"Name", "Email", "Role", "Assigned At"}, memberRows)

	columnNames := make(map[string]string, len(columns))
	for _, column := range columns {
		columnNames[column.ID] = column.Name
	}
	taskRows := make([][]string, 0, len(tasks))
	for _, task := range tasks {
		dueDate := "-"
		if task.DueDate != nil {
			dueDate = exportutil.FormatDate(*task.DueDate)
		}
		taskRows = append(taskRows, []string{
			task.Title,
			columnNames[task.ColumnID],
			exportutil.OptionalString(task.AssigneeName, "Unassigned"),
			dueDate,
			task.Priority,
		})
	}
	report.AddSection("Tasks")
	report.AddTable([]string{"Task", "Status", "Assignee", "Due Date", "Priority"}, taskRows)

	var buffer bytes.Buffer
	if err := report.Save(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func countProjectsByStatus(items []model.Project, status string) int {
	count := 0
	for _, item := range items {
		if strings.EqualFold(item.Status, status) {
			count++
		}
	}
	return count
}

func countProjectsByPriority(items []model.Project, priority string) int {
	count := 0
	for _, item := range items {
		if strings.EqualFold(item.Priority, priority) {
			count++
		}
	}
	return count
}

