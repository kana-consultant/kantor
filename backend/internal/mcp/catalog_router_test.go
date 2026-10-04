package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/app"
	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/mcp"
)

type countingExecutor struct{ calls int }

func (e *countingExecutor) Execute(*http.Request) (int, []byte, error) {
	e.calls++
	return http.StatusOK, []byte(`{"success":true}`), nil
}

// sensitiveRouteFragments must never appear in the MCP tool surface. Each
// phase that adds one of these route families adds its fragment here (and to
// excludedContains in catalog.go).
var sensitiveRouteFragments = []string{
	"/settings/document-mail",
	"/settings/mail-delivery",
	"/email-deliveries",
	"/hr-profile",
	"/settings/company-profile",
}

// renderedDocumentSuffixes are the binary payslip/contract files: the PKWT
// prints the full NIK and account number, so they never become tools.
var renderedDocumentSuffixes = []string{"/pdf", "/docx"}

// documentWorkflowTools are the payslip and contract tools an AI client gets.
// The value says whether the tool e-mails an employee and so needs
// confirm=true.
var documentWorkflowTools = map[string]bool{
	"get_hris_payslips":                         false,
	"post_hris_payslips_generate":               false,
	"post_hris_payslips_send_preview":           false,
	"post_hris_payslips_send_batch":             true,
	"get_hris_payslips_employee_employeeid":     false,
	"get_hris_payslips_payslipid":               false,
	"put_hris_payslips_payslipid":               false,
	"get_hris_payslips_payslipid_recipient":     false,
	"post_hris_payslips_payslipid_send":         true,
	"post_hris_payslips_payslipid_void_reissue": false,
	"get_hris_contracts":                        false,
	"post_hris_contracts":                       false,
	"get_hris_contracts_contractid":             false,
	"put_hris_contracts_contractid":             false,
	"get_hris_contracts_contractid_preflight":   false,
	"post_hris_contracts_contractid_generate":   false,
	"post_hris_contracts_contractid_renew":      false,
	"patch_hris_contracts_contractid_status":    false,
	"get_hris_contracts_contractid_recipient":   false,
	"post_hris_contracts_contractid_send":       true,
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
	byName := make(map[string]mcp.ToolSpec, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
		byName[tool.Name] = tool
		for _, fragment := range sensitiveRouteFragments {
			if strings.Contains(tool.PathTemplate, fragment) {
				t.Errorf("tool %s (%s %s) exposes excluded route fragment %q", tool.Name, tool.Method, tool.PathTemplate, fragment)
			}
		}
		for _, suffix := range renderedDocumentSuffixes {
			if strings.HasSuffix(tool.PathTemplate, suffix) {
				t.Errorf("tool %s (%s %s) exposes a rendered document", tool.Name, tool.Method, tool.PathTemplate)
			}
		}
	}

	// The payslip and contract workflow is on the surface, every tool with a
	// curated description, and exactly the e-mail sending tools need confirm.
	for name, sendsEmail := range documentWorkflowTools {
		tool, ok := byName[name]
		if !ok {
			t.Errorf("expected document workflow tool %s in the catalog", name)
			continue
		}
		if tool.Meta == nil || tool.Meta.Description == "" {
			t.Errorf("%s has no curated description (annotation key does not match the route?)", name)
			continue
		}
		if tool.Meta.RequireConfirm != sendsEmail {
			t.Errorf("%s: RequireConfirm = %v, want %v", name, tool.Meta.RequireConfirm, sendsEmail)
		}
		if (tool.Method == http.MethodPost || tool.Method == http.MethodPut || tool.Method == http.MethodPatch) && tool.Meta.Body == "" {
			t.Errorf("%s takes a body but its fields are not documented", name)
		}
	}
	// Irreversible document actions are flagged for clients that ask before
	// running destructive tools.
	for _, name := range []string{
		"post_hris_payslips_send_batch",
		"post_hris_payslips_payslipid_send",
		"post_hris_contracts_contractid_send",
		"post_hris_payslips_payslipid_void_reissue",
		"patch_hris_contracts_contractid_status",
	} {
		if tool, ok := byName[name]; ok && (tool.Meta == nil || !tool.Meta.Destructive) {
			t.Errorf("%s must be marked Destructive", name)
		}
	}

	// The gate itself, for every gated tool of the real catalog: without
	// confirm=true the API is never called.
	executor := &countingExecutor{}
	server := mcp.NewServer(tools, executor, "", "test")
	gated := 0
	for _, tool := range tools {
		if tool.Meta == nil || !tool.Meta.RequireConfirm {
			continue
		}
		gated++
		arguments := map[string]interface{}{"body": map[string]interface{}{"ids": []string{"x"}, "expected_recipient": "a@b.test"}}
		for _, param := range tool.PathParams {
			arguments[param] = "11111111-1111-1111-1111-111111111111"
		}
		call := func(confirm interface{}) {
			if confirm != nil {
				arguments["confirm"] = confirm
			} else {
				delete(arguments, "confirm")
			}
			message, err := json.Marshal(map[string]interface{}{
				"jsonrpc": "2.0", "id": 1, "method": "tools/call",
				"params": map[string]interface{}{"name": tool.Name, "arguments": arguments},
			})
			if err != nil {
				t.Fatal(err)
			}
			server.Handle(context.Background(), message, "Bearer token", "tenant.example.com")
		}
		for _, unconfirmed := range []interface{}{nil, false, "true", 1} {
			call(unconfirmed)
			if executor.calls != 0 {
				t.Fatalf("%s reached the API with confirm=%v", tool.Name, unconfirmed)
			}
		}
		call(true)
		if executor.calls != 1 {
			t.Fatalf("%s did not reach the API with confirm=true", tool.Name)
		}
		executor.calls = 0
	}
	if gated != 3 {
		t.Errorf("expected 3 gated tools in the real catalog, got %d", gated)
	}

	for _, tool := range tools {
		if !strings.Contains(tool.PathTemplate, "/hris/payslips") && !strings.Contains(tool.PathTemplate, "/hris/contracts") {
			if tool.Meta != nil && tool.Meta.RequireConfirm {
				t.Errorf("unexpected confirm gate on %s", tool.Name)
			}
			continue
		}
		if _, ok := documentWorkflowTools[tool.Name]; !ok {
			t.Errorf("new payslip/contract tool %s is not in documentWorkflowTools: decide whether it needs confirm=true and add it", tool.Name)
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
