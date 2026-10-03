package mcp_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/app"
	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/mcp"
)

// sensitiveRouteFragments must never appear in the MCP tool surface. Each
// phase that adds one of these route families adds its fragment here (and to
// excludedContains in catalog.go).
var sensitiveRouteFragments = []string{
	"/settings/document-mail",
	"/settings/mail-delivery",
	"/email-deliveries",
	"/hr-profile",
	"/settings/company-profile",
	"/hris/payslips",
	"/hris/contracts",
}

// TestCatalogFromRealRouterExcludesSensitiveRoutes builds the catalog from the
// application's real router (not a hand-made one), so a new route that slips
// past the exclusion list fails here.
func TestCatalogFromRealRouterExcludesSensitiveRoutes(t *testing.T) {
	routes, err := app.BuildRouteTableForInspection(config.Config{AppEnv: "test"})
	if err != nil {
		t.Fatalf("build real router: %v", err)
	}

	tools, err := mcp.BuildCatalog(routes)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	if len(tools) < 50 {
		t.Fatalf("catalog from the real router looks truncated: %d tools", len(tools))
	}

	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
		for _, fragment := range sensitiveRouteFragments {
			if strings.Contains(tool.PathTemplate, fragment) {
				t.Errorf("tool %s (%s %s) exposes excluded route fragment %q", tool.Name, tool.Method, tool.PathTemplate, fragment)
			}
		}
	}

	// Ordinary admin settings and employee tools stay available.
	for _, want := range []string{"get_admin_settings", "put_admin_settings_default_roles", "get_hris_employees_employeeid"} {
		if !names[want] {
			t.Errorf("expected %s to remain in the catalog", want)
		}
	}
	if names["put_admin_settings_mail_delivery"] {
		t.Error("put_admin_settings_mail_delivery must no longer be an MCP tool")
	}

	// Guard against a vacuous pass: the excluded routes must really be
	// mounted on the router the catalog was built from.
	mounted := map[string]bool{}
	walkErr := chi.Walk(routes, func(method string, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk router: %v", walkErr)
	}
	for _, route := range []string{
		"GET /api/v1/admin/settings/document-mail",
		"PUT /api/v1/admin/settings/document-mail",
		"POST /api/v1/admin/settings/document-mail/test",
		"PUT /api/v1/admin/settings/mail-delivery",
		"GET /api/v1/admin/settings/company-profile",
		"PUT /api/v1/admin/settings/company-profile",
		"GET /api/v1/admin/settings/company-profile/logo",
		"POST /api/v1/admin/settings/company-profile/logo",
		"DELETE /api/v1/admin/settings/company-profile/logo",
		"GET /api/v1/hris/employees/{employeeID}/hr-profile",
		"PUT /api/v1/hris/employees/{employeeID}/hr-profile",
		"GET /api/v1/hris/payslips",
		"POST /api/v1/hris/payslips/generate",
		"POST /api/v1/hris/payslips/send-preview",
		"POST /api/v1/hris/payslips/send-batch",
		"GET /api/v1/hris/payslips/employee/{employeeID}",
		"GET /api/v1/hris/payslips/{payslipID}",
		"PUT /api/v1/hris/payslips/{payslipID}",
		"GET /api/v1/hris/payslips/{payslipID}/pdf",
		"GET /api/v1/hris/payslips/{payslipID}/docx",
		"GET /api/v1/hris/payslips/{payslipID}/recipient",
		"POST /api/v1/hris/payslips/{payslipID}/send",
		"POST /api/v1/hris/payslips/{payslipID}/void-reissue",
		"GET /api/v1/hris/contracts",
		"POST /api/v1/hris/contracts",
		"GET /api/v1/hris/contracts/{contractID}",
		"PUT /api/v1/hris/contracts/{contractID}",
		"GET /api/v1/hris/contracts/{contractID}/preflight",
		"POST /api/v1/hris/contracts/{contractID}/generate",
		"POST /api/v1/hris/contracts/{contractID}/renew",
		"PATCH /api/v1/hris/contracts/{contractID}/status",
		"GET /api/v1/hris/contracts/{contractID}/files/{part}/pdf",
		"GET /api/v1/hris/contracts/{contractID}/files/{part}/docx",
		"GET /api/v1/hris/contracts/{contractID}/recipient",
		"POST /api/v1/hris/contracts/{contractID}/send",
		"GET /api/v1/hris/email-deliveries",
	} {
		if !mounted[route] {
			t.Errorf("expected %s to be mounted on the real router", route)
		}
	}
}
