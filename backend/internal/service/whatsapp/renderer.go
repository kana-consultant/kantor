package whatsapp

import (
	"regexp"
	"strings"
)

var unresolvedVarRegex = regexp.MustCompile(`\{\{[^}]+\}\}`)

// RenderTemplate replaces {{key}} placeholders with values from vars.
// Unresolved placeholders are removed. The result is trimmed.
func RenderTemplate(bodyTemplate string, vars map[string]string) string {
	result := bodyTemplate
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	result = unresolvedVarRegex.ReplaceAllString(result, "")
	result = strings.TrimSpace(result)
	return result
}

// BuildReviewerNotesSection returns the formatted reviewer notes line,
// or an empty string if notes is empty.
func BuildReviewerNotesSection(notes string) string {
	trimmed := strings.TrimSpace(notes)
	if trimmed == "" {
		return ""
	}
	return "📎 Catatan: " + trimmed
}

// SampleVars returns sample data for template preview.
func SampleVars(appURL string) map[string]string {
	return map[string]string{
		"name":                   "Ahmad Fauzi",
		"task_title":             "Buat landing page",
		"project_name":           "Project Alpha",
		"due_date":               "2026-03-25",
		"priority":               "high",
		"deadline":               "2026-03-25",
		"project_status":         "active",
		"open_tasks_count":       "5",
		"total_tasks_count":      "12",
		"week_start":             "2026-03-09",
		"week_end":               "2026-03-15",
		"completed_count":        "8",
		"open_count":             "3",
		"overdue_count":          "1",
		"reimbursement_title":    "Transport Meeting Client",
		"amount":                 "Rp 250.000",
		"new_status":             "approved",
		"reviewer_notes_section": "📎 Catatan: Sudah sesuai policy",
		"app_url":                appURL,
	}
}
