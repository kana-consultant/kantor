package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	serverName       = "kantor-mcp"
	serverVersion    = "0.1.0"
	protocolVersion  = "2024-11-05"
	defaultBaseURL   = "http://localhost:8080"
	maxResponseBytes = 2 << 20
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type apiRequestArgs struct {
	Method         string                 `json:"method"`
	Path           string                 `json:"path"`
	Query          map[string]interface{} `json:"query"`
	Body           interface{}            `json:"body"`
	Headers        map[string]string      `json:"headers"`
	AccessToken    string                 `json:"access_token"`
	TenantHost     string                 `json:"tenant_host"`
	TimeoutSeconds int                    `json:"timeout_seconds"`
}

type route struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Auth        string `json:"auth"`
	Description string `json:"description,omitempty"`
}

func main() {
	s := &server{
		baseURL:    env("KANTOR_API_BASE_URL", defaultBaseURL),
		token:      os.Getenv("KANTOR_API_TOKEN"),
		tenantHost: os.Getenv("KANTOR_TENANT_HOST"),
		client:     &http.Client{Timeout: 30 * time.Second},
	}

	scanner := bufio.NewScanner(os.Stdin)
	buffer := make([]byte, 0, 1024*1024)
	scanner.Buffer(buffer, 8*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeResponse(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}

		resp, ok := s.handle(context.Background(), req)
		if ok {
			writeResponse(resp)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type server struct {
	baseURL    string
	token      string
	tenantHost string
	client     *http.Client
}

func (s *server) handle(ctx context.Context, req rpcRequest) (rpcResponse, bool) {
	if strings.HasPrefix(req.Method, "notifications/") {
		return rpcResponse{}, false
	}

	result, err := s.dispatch(ctx, req)
	resp := rpcResponse{JSONRPC: "2.0", ID: rawID(req.ID)}
	if err != nil {
		resp.Error = &rpcError{Code: -32603, Message: err.Error()}
		return resp, true
	}
	resp.Result = result
	return resp, true
}

func (s *server) dispatch(ctx context.Context, req rpcRequest) (interface{}, error) {
	switch req.Method {
	case "initialize":
		return map[string]interface{}{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    serverName,
				"version": serverVersion,
			},
		}, nil
	case "tools/list":
		return map[string]interface{}{"tools": []interface{}{apiRequestTool()}}, nil
	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid tools/call params: %w", err)
		}
		if params.Name != "kantor_api_request" {
			return nil, fmt.Errorf("unknown tool %q", params.Name)
		}
		return s.callAPI(ctx, params.Arguments)
	case "resources/list":
		return map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"uri":         "kantor://api/routes",
					"name":        "KANTOR API route catalog",
					"description": "Known KANTOR HTTP API endpoints grouped by route path.",
					"mimeType":    "application/json",
				},
				map[string]interface{}{
					"uri":         "kantor://api/config",
					"name":        "KANTOR MCP runtime config",
					"description": "Effective API base URL and tenant host used by the MCP gateway.",
					"mimeType":    "application/json",
				},
			},
		}, nil
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid resources/read params: %w", err)
		}
		return s.readResource(params.URI)
	default:
		return nil, fmt.Errorf("unsupported method %q", req.Method)
	}
}

func (s *server) callAPI(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	var args apiRequestArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid kantor_api_request arguments: %w", err)
		}
	}

	method := strings.ToUpper(strings.TrimSpace(args.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !allowedMethod(method) {
		return nil, fmt.Errorf("unsupported HTTP method %q", method)
	}

	targetURL, err := s.buildURL(args.Path, args.Query)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if args.Body != nil {
		payload, err := json.Marshal(args.Body)
		if err != nil {
			return nil, fmt.Errorf("encode body: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	timeout := 30 * time.Second
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if args.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("Accept", "application/json")
	for key, value := range args.Headers {
		if strings.EqualFold(key, "Host") || strings.EqualFold(key, "Authorization") {
			continue
		}
		httpReq.Header.Set(key, value)
	}

	token := strings.TrimSpace(args.AccessToken)
	if token == "" {
		token = s.token
	}
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(token, "Bearer "))
	}

	tenantHost := strings.TrimSpace(args.TenantHost)
	if tenantHost == "" {
		tenantHost = s.tenantHost
	}
	if tenantHost != "" {
		httpReq.Host = tenantHost
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call KANTOR API: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	truncated := len(payload) > maxResponseBytes
	if truncated {
		payload = payload[:maxResponseBytes]
	}

	apiResp := map[string]interface{}{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"headers":    responseHeaders(resp.Header),
		"truncated":  truncated,
	}

	contentType := resp.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.Contains(mediaType, "json") {
		var decoded interface{}
		if err := json.Unmarshal(payload, &decoded); err == nil {
			apiResp["body"] = decoded
		} else {
			apiResp["body"] = string(payload)
		}
	} else if strings.HasPrefix(mediaType, "text/") || mediaType == "" {
		apiResp["body"] = string(payload)
	} else {
		apiResp["body_base64"] = base64.StdEncoding.EncodeToString(payload)
	}

	text, err := json.MarshalIndent(apiResp, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode tool response: %w", err)
	}

	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": string(text),
			},
		},
		"isError": resp.StatusCode >= 400,
	}, nil
}

func (s *server) buildURL(path string, query map[string]interface{}) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path is required")
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return "", errors.New("path must be relative, not an absolute URL")
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}

	base, err := url.Parse(s.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid KANTOR_API_BASE_URL: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return "", errors.New("KANTOR_API_BASE_URL must include scheme and host")
	}

	ref := &url.URL{Path: trimmed}
	target := base.ResolveReference(ref)
	values := target.Query()
	for key, value := range query {
		addQuery(values, key, value)
	}
	target.RawQuery = values.Encode()
	return target.String(), nil
}

func (s *server) readResource(uri string) (interface{}, error) {
	var text []byte
	var err error
	switch uri {
	case "kantor://api/routes":
		text, err = json.MarshalIndent(routes(), "", "  ")
	case "kantor://api/config":
		text, err = json.MarshalIndent(map[string]string{
			"base_url":    s.baseURL,
			"tenant_host": s.tenantHost,
		}, "", "  ")
	default:
		return nil, fmt.Errorf("unknown resource %q", uri)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{
				"uri":      uri,
				"mimeType": "application/json",
				"text":     string(text),
			},
		},
	}, nil
}

func apiRequestTool() map[string]interface{} {
	return map[string]interface{}{
		"name":        "kantor_api_request",
		"description": "Call any KANTOR HTTP API endpoint through the configured backend. Use a relative path such as /api/v1/hris/employees.",
		"inputSchema": map[string]interface{}{
			"type":     "object",
			"required": []string{"path"},
			"properties": map[string]interface{}{
				"method": map[string]interface{}{
					"type":        "string",
					"description": "HTTP method. Defaults to GET.",
					"enum":        []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Relative KANTOR API path, for example /api/v1/operational/projects.",
				},
				"query": map[string]interface{}{
					"type":                 "object",
					"description":          "Query string parameters. Values may be strings, numbers, booleans, or arrays.",
					"additionalProperties": true,
				},
				"body": map[string]interface{}{
					"description": "JSON request body for POST, PUT, and PATCH requests.",
				},
				"headers": map[string]interface{}{
					"type":                 "object",
					"description":          "Extra request headers. Authorization and Host are managed separately.",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"access_token": map[string]interface{}{
					"type":        "string",
					"description": "Optional JWT access token. Defaults to KANTOR_API_TOKEN when set.",
				},
				"tenant_host": map[string]interface{}{
					"type":        "string",
					"description": "Optional tenant Host header. Defaults to KANTOR_TENANT_HOST when set.",
				},
				"timeout_seconds": map[string]interface{}{
					"type":        "integer",
					"description": "Per-request timeout. Defaults to 30 seconds.",
					"minimum":     1,
				},
			},
		},
	}
}

func routes() []route {
	items := []route{
		{"GET", "/healthz", "public", "Backend process health check."},
		{"GET", "/readyz", "public", "Backend readiness and database check."},
		{"GET", "/api/v1/health", "tenant", "Tenant-scoped API health check."},
		{"POST", "/api/v1/auth/register", "tenant", "Register a user when registration is enabled."},
		{"POST", "/api/v1/auth/login", "tenant", "Issue access and refresh tokens."},
		{"GET", "/api/v1/auth/public-options", "tenant", "Read public auth options."},
		{"POST", "/api/v1/auth/forgot-password", "tenant", "Start password reset."},
		{"GET", "/api/v1/auth/reset-password/validate", "tenant", "Validate reset token."},
		{"POST", "/api/v1/auth/reset-password", "tenant", "Complete password reset."},
		{"POST", "/api/v1/auth/refresh", "tenant", "Refresh access token."},
		{"POST", "/api/v1/auth/logout", "tenant", "Logout current token/session."},
		{"GET", "/api/v1/auth/me", "jwt", "Current principal."},
		{"PUT", "/api/v1/auth/client-context", "jwt", "Update client context."},
		{"GET", "/api/v1/auth/profile", "jwt", "Current user profile."},
		{"PUT", "/api/v1/auth/profile", "jwt", "Update profile."},
		{"PUT", "/api/v1/auth/profile/email", "jwt", "Change profile email."},
		{"POST", "/api/v1/auth/profile/avatar", "jwt", "Upload profile avatar."},
		{"POST", "/api/v1/auth/change-password", "jwt", "Change password."},
		{"GET", "/api/v1/files/{type}/{id}/{filename}", "jwt", "Serve protected upload."},
		{"GET", "/api/v1/modules", "jwt", "List visible modules."},
		{"GET", "/api/v1/notifications", "jwt", "List notifications."},
		{"GET", "/api/v1/notifications/stream", "jwt", "Notification SSE stream."},
		{"GET", "/api/v1/notifications/unread-count", "jwt", "Unread notification count."},
		{"PATCH", "/api/v1/notifications/{notificationID}/read", "jwt", "Mark notification read."},
		{"PATCH", "/api/v1/notifications/read-all", "jwt", "Mark all notifications read."},
		{"GET", "/api/v1/admin/audit-logs", "jwt", "List audit logs."},
		{"GET", "/api/v1/admin/audit-logs/summary", "jwt", "Audit log summary."},
		{"GET", "/api/v1/admin/audit-logs/users", "jwt", "List audit log actors."},
		{"GET", "/api/v1/admin/audit-logs/export", "jwt", "Export audit logs."},
		{"GET", "/api/v1/admin/roles", "jwt", "List roles."},
		{"GET", "/api/v1/admin/roles/{roleID}", "jwt", "Get role."},
		{"POST", "/api/v1/admin/roles", "jwt", "Create role."},
		{"PUT", "/api/v1/admin/roles/{roleID}", "jwt", "Update role."},
		{"DELETE", "/api/v1/admin/roles/{roleID}", "jwt", "Delete role."},
		{"PATCH", "/api/v1/admin/roles/{roleID}/toggle", "jwt", "Toggle role active state."},
		{"POST", "/api/v1/admin/roles/{roleID}/duplicate", "jwt", "Duplicate role."},
		{"GET", "/api/v1/admin/permissions", "jwt", "List permissions."},
		{"GET", "/api/v1/admin/users", "jwt", "List users."},
		{"GET", "/api/v1/admin/users/{userID}", "jwt", "Get user."},
		{"PUT", "/api/v1/admin/users/{userID}/roles", "jwt", "Update user roles."},
		{"PATCH", "/api/v1/admin/users/{userID}/active", "jwt", "Toggle user active state."},
		{"POST", "/api/v1/admin/users/{userID}/ensure-employee-profile", "jwt", "Ensure linked employee profile."},
		{"POST", "/api/v1/admin/users/{userID}/toggle-super-admin", "jwt", "Toggle super admin."},
		{"GET", "/api/v1/admin/settings", "jwt", "Read admin settings."},
		{"GET", "/api/v1/admin/settings/departments", "jwt", "List settings departments."},
		{"PUT", "/api/v1/admin/settings/default-roles", "jwt", "Update default roles."},
		{"PUT", "/api/v1/admin/settings/auto-create-employee", "jwt", "Update auto-create employee setting."},
		{"PUT", "/api/v1/admin/settings/mail-delivery", "jwt", "Update mail delivery setting."},
		{"PUT", "/api/v1/admin/settings/reimbursement-reminder", "jwt", "Update reimbursement reminder setting."},
		{"GET", "/api/v1/admin/settings/registration", "jwt", "Read registration settings."},
		{"PUT", "/api/v1/admin/settings/registration", "jwt", "Update registration settings."},
		{"POST", "/api/v1/admin/settings/registration/roll", "jwt", "Roll registration code."},
	}

	items = append(items, moduleRoutes()...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].Method < items[j].Method
		}
		return items[i].Path < items[j].Path
	})
	return items
}

func moduleRoutes() []route {
	return []route{
		{"GET", "/api/v1/operational/overview", "jwt", "Operational overview."},
		{"GET", "/api/v1/operational/projects", "jwt", "List projects."},
		{"POST", "/api/v1/operational/projects", "jwt", "Create project."},
		{"GET", "/api/v1/operational/projects/export", "jwt", "Export projects."},
		{"GET", "/api/v1/operational/projects/available-users", "jwt", "List assignable users."},
		{"GET", "/api/v1/operational/projects/{projectID}", "jwt", "Get project."},
		{"PUT", "/api/v1/operational/projects/{projectID}", "jwt", "Update project."},
		{"DELETE", "/api/v1/operational/projects/{projectID}", "jwt", "Delete project."},
		{"POST", "/api/v1/operational/projects/{projectID}/members", "jwt", "Mutate project members."},
		{"GET", "/api/v1/operational/projects/{projectID}/export", "jwt", "Export project detail."},
		{"GET", "/api/v1/operational/projects/{projectID}/columns", "jwt", "List project columns."},
		{"POST", "/api/v1/operational/projects/{projectID}/columns", "jwt", "Create project column."},
		{"PUT", "/api/v1/operational/projects/{projectID}/columns/{columnID}", "jwt", "Update project column."},
		{"DELETE", "/api/v1/operational/projects/{projectID}/columns/{columnID}", "jwt", "Delete project column."},
		{"PATCH", "/api/v1/operational/projects/{projectID}/columns/reorder", "jwt", "Reorder project columns."},
		{"GET", "/api/v1/operational/projects/{projectID}/tasks", "jwt", "List project tasks."},
		{"POST", "/api/v1/operational/projects/{projectID}/tasks", "jwt", "Create project task."},
		{"PUT", "/api/v1/operational/projects/{projectID}/tasks/{taskID}", "jwt", "Update project task."},
		{"DELETE", "/api/v1/operational/projects/{projectID}/tasks/{taskID}", "jwt", "Delete project task."},
		{"PATCH", "/api/v1/operational/projects/{projectID}/tasks/{taskID}/move", "jwt", "Move project task."},
		{"GET", "/api/v1/operational/vps", "jwt", "List VPS records."},
		{"POST", "/api/v1/operational/vps", "jwt", "Create VPS record."},
		{"GET", "/api/v1/operational/vps/{vpsID}", "jwt", "Get VPS detail."},
		{"PUT", "/api/v1/operational/vps/{vpsID}", "jwt", "Update VPS record."},
		{"DELETE", "/api/v1/operational/vps/{vpsID}", "jwt", "Delete VPS record."},
		{"POST", "/api/v1/operational/vps/{vpsID}/checks", "jwt", "Create VPS check."},
		{"PUT", "/api/v1/operational/vps/{vpsID}/checks/{checkID}", "jwt", "Update VPS check."},
		{"DELETE", "/api/v1/operational/vps/{vpsID}/checks/{checkID}", "jwt", "Delete VPS check."},
		{"POST", "/api/v1/operational/vps/{vpsID}/apps", "jwt", "Create VPS app."},
		{"PUT", "/api/v1/operational/vps/{vpsID}/apps/{appID}", "jwt", "Update VPS app."},
		{"DELETE", "/api/v1/operational/vps/{vpsID}/apps/{appID}", "jwt", "Delete VPS app."},
		{"GET", "/api/v1/operational/domains", "jwt", "List domains."},
		{"POST", "/api/v1/operational/domains", "jwt", "Create domain."},
		{"GET", "/api/v1/operational/domains/{domainID}", "jwt", "Get domain."},
		{"PUT", "/api/v1/operational/domains/{domainID}", "jwt", "Update domain."},
		{"DELETE", "/api/v1/operational/domains/{domainID}", "jwt", "Delete domain."},
		{"GET", "/api/v1/tracker/consent", "jwt", "Get tracker consent."},
		{"POST", "/api/v1/tracker/consent", "jwt", "Give tracker consent."},
		{"DELETE", "/api/v1/tracker/consent", "jwt", "Revoke tracker consent."},
		{"POST", "/api/v1/tracker/sessions/start", "jwt", "Start tracker session."},
		{"PATCH", "/api/v1/tracker/sessions/{sessionID}/end", "jwt", "End tracker session."},
		{"POST", "/api/v1/tracker/heartbeat", "jwt", "Submit tracker heartbeat."},
		{"POST", "/api/v1/tracker/entries/batch", "jwt", "Submit tracker entries in batch."},
		{"GET", "/api/v1/tracker/my-activity", "jwt", "Current user's tracker activity."},
		{"GET", "/api/v1/tracker/extension/download", "jwt", "Download tracker extension."},
		{"GET", "/api/v1/tracker/team-activity", "jwt", "Team tracker activity."},
		{"GET", "/api/v1/tracker/activity/{userID}", "jwt", "Tracker activity for a user."},
		{"GET", "/api/v1/tracker/summary", "jwt", "Tracker summary."},
		{"GET", "/api/v1/tracker/consents", "jwt", "Tracker consent audit list."},
		{"GET", "/api/v1/tracker/domains", "jwt", "List tracker domain rules."},
		{"POST", "/api/v1/tracker/domains", "jwt", "Create tracker domain rule."},
		{"PUT", "/api/v1/tracker/domains/{domainID}", "jwt", "Update tracker domain rule."},
		{"DELETE", "/api/v1/tracker/domains/{domainID}", "jwt", "Delete tracker domain rule."},
		{"GET", "/api/v1/tracker/domains/observed", "jwt", "List observed tracker domains."},
		{"POST", "/api/v1/tracker/domains/observed/bulk-classify", "jwt", "Bulk classify observed tracker domains."},
		{"GET", "/api/v1/tracker/reminder-config", "jwt", "Read tracker reminder config."},
		{"PUT", "/api/v1/tracker/reminder-config", "jwt", "Update tracker reminder config."},
		{"POST", "/api/v1/tracker/reminder-config/test", "jwt", "Send tracker reminder test."},
		{"GET", "/api/v1/hris/overview", "jwt", "HRIS overview."},
		{"GET", "/api/v1/hris/employees", "jwt", "List employees."},
		{"POST", "/api/v1/hris/employees", "jwt", "Create employee."},
		{"GET", "/api/v1/hris/employees/export", "jwt", "Export employees."},
		{"GET", "/api/v1/hris/employees/me", "jwt", "Current employee profile."},
		{"GET", "/api/v1/hris/employees/{employeeID}", "jwt", "Get employee."},
		{"PUT", "/api/v1/hris/employees/{employeeID}", "jwt", "Update employee."},
		{"DELETE", "/api/v1/hris/employees/{employeeID}", "jwt", "Delete employee."},
		{"POST", "/api/v1/hris/employees/{employeeID}/avatar", "jwt", "Upload employee avatar."},
		{"GET", "/api/v1/hris/employees/{employeeID}/export", "jwt", "Export employee detail."},
		{"GET", "/api/v1/hris/employees/{employeeID}/salaries", "jwt", "List salaries."},
		{"POST", "/api/v1/hris/employees/{employeeID}/salaries", "jwt", "Create salary."},
		{"GET", "/api/v1/hris/employees/{employeeID}/salaries/current", "jwt", "Get current salary."},
		{"GET", "/api/v1/hris/employees/{employeeID}/bonuses", "jwt", "List bonuses."},
		{"POST", "/api/v1/hris/employees/{employeeID}/bonuses", "jwt", "Create bonus."},
		{"GET", "/api/v1/hris/departments", "jwt", "List departments."},
		{"POST", "/api/v1/hris/departments", "jwt", "Create department."},
		{"GET", "/api/v1/hris/departments/{departmentID}", "jwt", "Get department."},
		{"PUT", "/api/v1/hris/departments/{departmentID}", "jwt", "Update department."},
		{"DELETE", "/api/v1/hris/departments/{departmentID}", "jwt", "Delete department."},
		{"PUT", "/api/v1/hris/bonuses/{bonusID}", "jwt", "Update bonus."},
		{"DELETE", "/api/v1/hris/bonuses/{bonusID}", "jwt", "Delete bonus."},
		{"PATCH", "/api/v1/hris/bonuses/{bonusID}/approve", "jwt", "Approve bonus."},
		{"PATCH", "/api/v1/hris/bonuses/{bonusID}/reject", "jwt", "Reject bonus."},
		{"GET", "/api/v1/hris/finance/categories", "jwt", "List finance categories."},
		{"POST", "/api/v1/hris/finance/categories", "jwt", "Create finance category."},
		{"PUT", "/api/v1/hris/finance/categories/{categoryID}", "jwt", "Update finance category."},
		{"DELETE", "/api/v1/hris/finance/categories/{categoryID}", "jwt", "Delete finance category."},
		{"GET", "/api/v1/hris/finance/records", "jwt", "List finance records."},
		{"POST", "/api/v1/hris/finance/records", "jwt", "Create finance record."},
		{"GET", "/api/v1/hris/finance/records/{recordID}", "jwt", "Get finance record."},
		{"PUT", "/api/v1/hris/finance/records/{recordID}", "jwt", "Update finance record."},
		{"DELETE", "/api/v1/hris/finance/records/{recordID}", "jwt", "Delete finance record."},
		{"PATCH", "/api/v1/hris/finance/records/{recordID}/submit", "jwt", "Submit finance record."},
		{"PATCH", "/api/v1/hris/finance/records/{recordID}/review", "jwt", "Review finance record."},
		{"GET", "/api/v1/hris/finance/summary", "jwt", "Finance summary."},
		{"GET", "/api/v1/hris/finance/export", "jwt", "Export finance records."},
		{"GET", "/api/v1/hris/reimbursements", "jwt", "List reimbursements."},
		{"POST", "/api/v1/hris/reimbursements", "jwt", "Create reimbursement."},
		{"GET", "/api/v1/hris/reimbursements/export", "jwt", "Export reimbursements."},
		{"GET", "/api/v1/hris/reimbursements/summary", "jwt", "Reimbursement summary."},
		{"GET", "/api/v1/hris/reimbursements/{reimbursementID}", "jwt", "Get reimbursement."},
		{"PUT", "/api/v1/hris/reimbursements/{reimbursementID}", "jwt", "Update reimbursement."},
		{"DELETE", "/api/v1/hris/reimbursements/{reimbursementID}", "jwt", "Delete reimbursement."},
		{"POST", "/api/v1/hris/reimbursements/{reimbursementID}/attachments", "jwt", "Upload reimbursement attachments."},
		{"PATCH", "/api/v1/hris/reimbursements/{reimbursementID}/review", "jwt", "Review reimbursement."},
		{"POST", "/api/v1/hris/reimbursements/bulk-review", "jwt", "Bulk review reimbursements."},
		{"PATCH", "/api/v1/hris/reimbursements/{reimbursementID}/mark-paid", "jwt", "Mark reimbursement paid."},
		{"POST", "/api/v1/hris/reimbursements/bulk-mark-paid", "jwt", "Bulk mark reimbursements paid."},
		{"GET", "/api/v1/hris/subscriptions", "jwt", "List subscriptions."},
		{"POST", "/api/v1/hris/subscriptions", "jwt", "Create subscription."},
		{"GET", "/api/v1/hris/subscriptions/export", "jwt", "Export subscriptions."},
		{"GET", "/api/v1/hris/subscriptions/summary", "jwt", "Subscription summary."},
		{"GET", "/api/v1/hris/subscriptions/alerts", "jwt", "List subscription alerts."},
		{"GET", "/api/v1/hris/subscriptions/{subscriptionID}", "jwt", "Get subscription."},
		{"PUT", "/api/v1/hris/subscriptions/{subscriptionID}", "jwt", "Update subscription."},
		{"DELETE", "/api/v1/hris/subscriptions/{subscriptionID}", "jwt", "Delete subscription."},
		{"PATCH", "/api/v1/hris/subscriptions/alerts/{alertID}/read", "jwt", "Mark subscription alert read."},
		{"GET", "/api/v1/marketing/overview", "jwt", "Marketing overview."},
		{"GET", "/api/v1/marketing/campaigns", "jwt", "List campaigns."},
		{"POST", "/api/v1/marketing/campaigns", "jwt", "Create campaign."},
		{"GET", "/api/v1/marketing/campaigns/export", "jwt", "Export campaigns."},
		{"GET", "/api/v1/marketing/campaigns/kanban", "jwt", "Campaign kanban."},
		{"GET", "/api/v1/marketing/campaigns/{campaignID}", "jwt", "Get campaign."},
		{"PUT", "/api/v1/marketing/campaigns/{campaignID}", "jwt", "Update campaign."},
		{"DELETE", "/api/v1/marketing/campaigns/{campaignID}", "jwt", "Delete campaign."},
		{"PATCH", "/api/v1/marketing/campaigns/{campaignID}/move", "jwt", "Move campaign."},
		{"GET", "/api/v1/marketing/campaigns/{campaignID}/activities", "jwt", "List campaign activities."},
		{"GET", "/api/v1/marketing/campaigns/{campaignID}/attachments", "jwt", "List campaign attachments."},
		{"POST", "/api/v1/marketing/campaigns/{campaignID}/attachments", "jwt", "Upload campaign attachment."},
		{"DELETE", "/api/v1/marketing/campaigns/{campaignID}/attachments/{attachmentID}", "jwt", "Delete campaign attachment."},
		{"GET", "/api/v1/marketing/columns", "jwt", "List marketing columns."},
		{"POST", "/api/v1/marketing/columns", "jwt", "Create marketing column."},
		{"PUT", "/api/v1/marketing/columns/{columnID}", "jwt", "Update marketing column."},
		{"DELETE", "/api/v1/marketing/columns/{columnID}", "jwt", "Delete marketing column."},
		{"PATCH", "/api/v1/marketing/columns/reorder", "jwt", "Reorder marketing columns."},
		{"GET", "/api/v1/marketing/ads-metrics", "jwt", "List ads metrics."},
		{"POST", "/api/v1/marketing/ads-metrics", "jwt", "Create ads metric."},
		{"POST", "/api/v1/marketing/ads-metrics/batch", "jwt", "Create ads metrics in batch."},
		{"GET", "/api/v1/marketing/ads-metrics/export", "jwt", "Export ads metrics."},
		{"GET", "/api/v1/marketing/ads-metrics/summary", "jwt", "Ads metric summary."},
		{"GET", "/api/v1/marketing/ads-metrics/{metricID}", "jwt", "Get ads metric."},
		{"PUT", "/api/v1/marketing/ads-metrics/{metricID}", "jwt", "Update ads metric."},
		{"DELETE", "/api/v1/marketing/ads-metrics/{metricID}", "jwt", "Delete ads metric."},
		{"GET", "/api/v1/marketing/leads", "jwt", "List leads."},
		{"POST", "/api/v1/marketing/leads", "jwt", "Create lead."},
		{"GET", "/api/v1/marketing/leads/export", "jwt", "Export leads."},
		{"GET", "/api/v1/marketing/leads/summary", "jwt", "Lead summary."},
		{"GET", "/api/v1/marketing/leads/pipeline", "jwt", "Lead pipeline."},
		{"POST", "/api/v1/marketing/leads/import", "jwt", "Import leads from CSV."},
		{"GET", "/api/v1/marketing/leads/{leadID}", "jwt", "Get lead."},
		{"PUT", "/api/v1/marketing/leads/{leadID}", "jwt", "Update lead."},
		{"DELETE", "/api/v1/marketing/leads/{leadID}", "jwt", "Delete lead."},
		{"PATCH", "/api/v1/marketing/leads/{leadID}/status", "jwt", "Move lead status."},
		{"GET", "/api/v1/marketing/leads/{leadID}/activities", "jwt", "List lead activities."},
		{"POST", "/api/v1/marketing/leads/{leadID}/activities", "jwt", "Create lead activity."},
		{"GET", "/api/v1/wa/config", "jwt", "Read WhatsApp config."},
		{"PUT", "/api/v1/wa/config", "jwt", "Update WhatsApp config."},
		{"GET", "/api/v1/wa/status", "jwt", "WhatsApp status."},
		{"GET", "/api/v1/wa/qr", "jwt", "WhatsApp QR."},
		{"POST", "/api/v1/wa/session/start", "jwt", "Start WhatsApp session."},
		{"POST", "/api/v1/wa/session/stop", "jwt", "Stop WhatsApp session."},
		{"GET", "/api/v1/wa/stats", "jwt", "WhatsApp stats."},
		{"GET", "/api/v1/wa/templates", "jwt", "List WhatsApp templates."},
		{"POST", "/api/v1/wa/templates/generate-defaults", "jwt", "Generate default WhatsApp templates."},
		{"POST", "/api/v1/wa/templates", "jwt", "Create WhatsApp template."},
		{"PUT", "/api/v1/wa/templates/{templateID}", "jwt", "Update WhatsApp template."},
		{"DELETE", "/api/v1/wa/templates/{templateID}", "jwt", "Delete WhatsApp template."},
		{"POST", "/api/v1/wa/templates/{templateID}/preview", "jwt", "Preview WhatsApp template."},
		{"GET", "/api/v1/wa/schedules", "jwt", "List WhatsApp schedules."},
		{"POST", "/api/v1/wa/schedules", "jwt", "Create WhatsApp schedule."},
		{"PUT", "/api/v1/wa/schedules/{scheduleID}", "jwt", "Update WhatsApp schedule."},
		{"DELETE", "/api/v1/wa/schedules/{scheduleID}", "jwt", "Delete WhatsApp schedule."},
		{"POST", "/api/v1/wa/schedules/{scheduleID}/trigger", "jwt", "Trigger WhatsApp schedule."},
		{"PATCH", "/api/v1/wa/schedules/{scheduleID}/toggle", "jwt", "Toggle WhatsApp schedule."},
		{"GET", "/api/v1/wa/logs", "jwt", "List WhatsApp logs."},
		{"GET", "/api/v1/wa/logs/summary", "jwt", "WhatsApp log summary."},
		{"POST", "/api/v1/wa/send", "jwt", "Send quick WhatsApp message."},
		{"GET", "/api/v1/wa/phone", "jwt", "Get user phone."},
		{"PUT", "/api/v1/wa/phone", "jwt", "Update user phone."},
	}
}

func allowedMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func addQuery(values url.Values, key string, value interface{}) {
	if key == "" || value == nil {
		return
	}
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			addQuery(values, key, item)
		}
	case []string:
		for _, item := range typed {
			values.Add(key, item)
		}
	case string:
		values.Add(key, typed)
	case bool:
		values.Add(key, strconv.FormatBool(typed))
	case float64:
		values.Add(key, strconv.FormatFloat(typed, 'f', -1, 64))
	default:
		values.Add(key, fmt.Sprint(typed))
	}
}

func responseHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		result[key] = strings.Join(values, ", ")
	}
	return result
}

func rawID(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var id interface{}
	if err := json.Unmarshal(raw, &id); err != nil {
		return string(raw)
	}
	return id
}

func writeResponse(resp rpcResponse) {
	encoded, err := json.Marshal(resp)
	if err != nil {
		encoded, _ = json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      resp.ID,
			Error:   &rpcError{Code: -32603, Message: "failed to encode response"},
		})
	}
	fmt.Println(string(encoded))
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
