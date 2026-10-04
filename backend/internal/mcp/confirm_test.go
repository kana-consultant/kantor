package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/clientvia"
)

func sendTool(t *testing.T) ToolSpec {
	t.Helper()
	const route = "/api/v1/hris/payslips/{payslipID}/send"
	meta := annotationFor(http.MethodPost, route)
	if meta == nil {
		t.Fatalf("expected annotation for POST %s", route)
	}
	return ToolSpec{
		Name:         toolName(http.MethodPost, route),
		Method:       http.MethodPost,
		PathTemplate: route,
		PathParams:   pathParams(route),
		Meta:         meta,
	}
}

func callSend(t *testing.T, server *Server, arguments string) (isError bool, text string) {
	t.Helper()
	message := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"post_hris_payslips_payslipid_send","arguments":` + arguments + `}}`)
	encoded, _ := server.Handle(context.Background(), message, "Bearer token", "tenant.example.com")

	var parsed struct {
		Error  *rpcError `json:"error"`
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatalf("response not valid json: %v", err)
	}
	if parsed.Error != nil {
		t.Fatalf("unexpected protocol error: %+v", parsed.Error)
	}
	if len(parsed.Result.Content) == 0 {
		t.Fatalf("tool result has no content: %s", encoded)
	}
	return parsed.Result.IsError, parsed.Result.Content[0].Text
}

func TestSendToolRefusesWithoutConfirm(t *testing.T) {
	for name, arguments := range map[string]string{
		"missing":        `{"payslipID":"p-1","body":{}}`,
		"false":          `{"payslipID":"p-1","confirm":false}`,
		"string true":    `{"payslipID":"p-1","confirm":"true"}`,
		"number one":     `{"payslipID":"p-1","confirm":1}`,
		"null":           `{"payslipID":"p-1","confirm":null}`,
		"inside body":    `{"payslipID":"p-1","body":{"confirm":true}}`,
		"inside query":   `{"payslipID":"p-1","query":{"confirm":true}}`,
		"no path either": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			executor := &stubExecutor{status: http.StatusOK, body: []byte(`{"success":true}`)}
			server := NewServer([]ToolSpec{sendTool(t)}, executor, "", "test")

			isError, text := callSend(t, server, arguments)
			if executor.lastRequest != nil {
				t.Fatalf("the API was called without confirm=true (%s)", arguments)
			}
			if !isError {
				t.Error("expected an error tool result")
			}
			if !strings.Contains(text, "confirm=true") || !strings.Contains(text, "Nothing was sent") {
				t.Errorf("the refusal must tell the model what to do, got %q", text)
			}
		})
	}
}

func TestSendToolRunsWithConfirm(t *testing.T) {
	executor := &stubExecutor{status: http.StatusOK, body: []byte(`{"success":true}`)}
	server := NewServer([]ToolSpec{sendTool(t)}, executor, "", "test")

	isError, _ := callSend(t, server, `{"payslipID":"p-1","confirm":true,"body":{"recipient_source":"login"}}`)
	if isError {
		t.Fatal("confirmed call must not be an error result")
	}
	if executor.lastRequest == nil {
		t.Fatal("confirmed call did not reach the API")
	}
	if got := executor.lastRequest.URL.Path; got != "/api/v1/hris/payslips/p-1/send" {
		t.Errorf("unexpected path %q", got)
	}
	if got := executor.lastRequest.URL.RawQuery; got != "" {
		t.Errorf("confirm must not be forwarded as a query parameter, got %q", got)
	}
	body, err := io.ReadAll(executor.lastRequest.Body)
	if err != nil {
		t.Fatalf("read forwarded body: %v", err)
	}
	if strings.Contains(string(body), "confirm") {
		t.Errorf("confirm must not be forwarded in the body, got %s", body)
	}
	if !strings.Contains(string(body), `"recipient_source":"login"`) {
		t.Errorf("body not forwarded, got %s", body)
	}
	// The API applies its MCP rules (login address only, expected_recipient)
	// to requests carrying this marker.
	if got := executor.lastRequest.Header.Get(clientvia.Header); got != clientvia.MCP {
		t.Errorf("%s = %q, want %q", clientvia.Header, got, clientvia.MCP)
	}
}

// Every request the MCP server builds is marked, whatever the tool.
func TestBuildRequestMarksMCP(t *testing.T) {
	for _, tool := range []ToolSpec{
		{Name: "get_x", Method: http.MethodGet, PathTemplate: "/api/v1/x"},
		{Name: "post_x", Method: http.MethodPost, PathTemplate: "/api/v1/x"},
		sendTool(t),
	} {
		args := map[string]json.RawMessage{"payslipID": json.RawMessage(`"p-1"`)}
		req, err := tool.buildRequest(context.Background(), "", args, "Bearer token", "tenant.example.com")
		if err != nil {
			t.Fatalf("%s: buildRequest: %v", tool.Name, err)
		}
		if !clientvia.IsMCP(req) {
			t.Errorf("%s: request is not marked as coming through MCP", tool.Name)
		}
	}
}

func TestUnconfirmedToolsAreNotGated(t *testing.T) {
	const route = "/api/v1/hris/payslips/send-preview"
	tool := ToolSpec{
		Name:         toolName(http.MethodPost, route),
		Method:       http.MethodPost,
		PathTemplate: route,
		Meta:         annotationFor(http.MethodPost, route),
	}
	executor := &stubExecutor{status: http.StatusOK, body: []byte(`{"success":true}`)}
	server := NewServer([]ToolSpec{tool}, executor, "", "test")

	message := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"post_hris_payslips_send_preview","arguments":{"body":{"ids":["a"]}}}}`)
	server.Handle(context.Background(), message, "Bearer token", "")
	if executor.lastRequest == nil {
		t.Fatal("send-preview sends nothing and must not need confirm")
	}
}

func TestSendToolDescriptor(t *testing.T) {
	descriptor := sendTool(t).descriptor()

	annotations := descriptor["annotations"].(map[string]interface{})
	if annotations["destructiveHint"] != true {
		t.Errorf("a send tool must be destructiveHint=true, got %v", annotations["destructiveHint"])
	}
	if annotations["openWorldHint"] != true {
		t.Errorf("a send tool must be openWorldHint=true, got %v", annotations["openWorldHint"])
	}
	if annotations["readOnlyHint"] != false {
		t.Errorf("a send tool must not be read-only, got %v", annotations["readOnlyHint"])
	}

	schema := descriptor["inputSchema"].(map[string]interface{})
	properties := schema["properties"].(map[string]interface{})
	confirm, ok := properties[confirmArg].(map[string]interface{})
	if !ok || confirm["type"] != "boolean" {
		t.Fatalf("send tool schema must have a boolean confirm property, got %v", properties[confirmArg])
	}
	required := schema["required"].([]string)
	if !contains(required, confirmArg) || !contains(required, "payslipID") {
		t.Errorf("confirm and payslipID must be required, got %v", required)
	}
	body := properties["body"].(map[string]interface{})["description"].(string)
	if !strings.Contains(body, "expected_recipient") {
		t.Errorf("body description should document the fields, got %q", body)
	}
	if !strings.Contains(descriptor["description"].(string), "SENDS E-MAIL") {
		t.Errorf("description should warn that the tool sends e-mail, got %q", descriptor["description"])
	}
}

// Tools without the new metadata keep the descriptor they had: no confirm
// property, no openWorldHint key, the generic body text.
func TestPlainToolDescriptorUnchanged(t *testing.T) {
	tool := ToolSpec{Name: "post_hris_departments", Method: http.MethodPost, PathTemplate: "/api/v1/hris/departments"}
	descriptor := tool.descriptor()

	annotations := descriptor["annotations"].(map[string]interface{})
	if _, has := annotations["openWorldHint"]; has {
		t.Error("openWorldHint must only appear on open-world tools")
	}
	if annotations["destructiveHint"] != false {
		t.Errorf("a plain POST must stay destructiveHint=false, got %v", annotations["destructiveHint"])
	}
	properties := descriptor["inputSchema"].(map[string]interface{})["properties"].(map[string]interface{})
	if _, has := properties[confirmArg]; has {
		t.Error("confirm must only appear on RequireConfirm tools")
	}
	if got := properties["body"].(map[string]interface{})["description"]; got != "JSON request body for this endpoint." {
		t.Errorf("plain body description changed: %q", got)
	}
}

// Among the curated annotations, exactly the three document send tools are
// gated (the real-router test checks the same on the built catalog).
func TestOnlyDocumentSendToolsRequireConfirm(t *testing.T) {
	want := map[string]bool{
		"POST /api/v1/hris/payslips/send-batch":         true,
		"POST /api/v1/hris/payslips/{payslipID}/send":   true,
		"POST /api/v1/hris/contracts/{contractID}/send": true,
	}
	for key, meta := range endpointAnnotations {
		if meta.RequireConfirm != want[key] {
			t.Errorf("%s: RequireConfirm = %v, want %v", key, meta.RequireConfirm, want[key])
		}
		if meta.RequireConfirm && (!meta.Destructive || !meta.OpenWorld) {
			t.Errorf("%s: a confirmed send tool must also be Destructive and OpenWorld", key)
		}
		if meta.OpenWorld && !meta.RequireConfirm {
			t.Errorf("%s: an open-world tool must require confirm", key)
		}
	}
	for key := range want {
		if _, ok := endpointAnnotations[key]; !ok {
			t.Errorf("missing annotation for %s", key)
		}
	}
}

func TestBuildCatalogDocumentWorkflowWithoutRenderedFiles(t *testing.T) {
	router := chi.NewRouter()
	h := noopHandler()
	router.Route("/api/v1", func(api chi.Router) {
		api.Get("/hris/payslips", h)
		api.Post("/hris/payslips/send-batch", h)
		api.Post("/hris/payslips/{payslipID}/send", h)
		api.Get("/hris/payslips/{payslipID}/pdf", h)
		api.Get("/hris/payslips/{payslipID}/docx", h)
		api.Get("/hris/contracts/{contractID}", h)
		api.Post("/hris/contracts/{contractID}/send", h)
		api.Get("/hris/contracts/{contractID}/files/{part}/pdf", h)
		api.Get("/hris/contracts/{contractID}/files/{part}/docx", h)
		api.Get("/hris/employees/{employeeID}/hr-profile", h)
		api.Get("/hris/email-deliveries", h)
	})

	tools, err := BuildCatalog(router)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		"get_hris_payslips",
		"post_hris_payslips_send_batch",
		"post_hris_payslips_payslipid_send",
		"get_hris_contracts_contractid",
		"post_hris_contracts_contractid_send",
	} {
		if !names[want] {
			t.Errorf("expected %s to be a tool", want)
		}
	}
	for _, excluded := range []string{
		"get_hris_payslips_payslipid_pdf",
		"get_hris_payslips_payslipid_docx",
		"get_hris_contracts_contractid_files_part_pdf",
		"get_hris_contracts_contractid_files_part_docx",
		"get_hris_employees_employeeid_hr_profile",
		"get_hris_email_deliveries",
	} {
		if names[excluded] {
			t.Errorf("expected %s to stay off the tool surface", excluded)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
