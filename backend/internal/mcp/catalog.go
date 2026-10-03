package mcp

import (
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

const apiPrefix = "/api/v1/"

// excludedSuffixes drops superadmin-only and public/unauthenticated endpoints
// from the tool surface. Superadmin features are intentionally never exposed to
// AI clients; public auth flows make no sense for a token-bound agent.
var excludedSuffixes = []string{
	"/users/{userID}/toggle-super-admin",
	"/settings/registration",
	"/settings/registration/roll",
	"/auth/register",
	"/auth/login",
	"/auth/logout",
	"/auth/refresh",
	"/auth/forgot-password",
	"/auth/reset-password",
	"/auth/reset-password/validate",
	"/auth/public-options",
	"/auth/change-password",
	"/notifications/stream",
	"/health",
	// Binary / full-dataset endpoints are useless (and huge) as text for an AI
	// client, and export endpoints force per_page=10000 — a fast way to blow up
	// the context window. Keep them off the tool surface.
	"/export",
	"/extension/download",
}

// excludedContains drops credential and OAuth self-management endpoints: an AI
// client must not mint or revoke its own tokens or approve OAuth grants. It also
// drops file-serving routes, which return binary blobs, and the mail
// credential / email-delivery routes: an AI client must not rewire where
// tenant or document email goes, send it, or read who was mailed what. HR
// document data stays off the surface too: employee identity (NIK, birth
// data) behind /hr-profile, the company profile + logo that every
// generated document carries, payslips (amounts, PDFs, sending) and
// employment contracts (PKWT/NDA with full NIK, account number and
// compensation; sending).
var excludedContains = []string{
	"/auth/pat",
	"/oauth",
	"/files/",
	"/settings/document-mail",
	"/settings/mail-delivery",
	"/email-deliveries",
	"/hr-profile",
	"/settings/company-profile",
	"/hris/payslips",
	"/hris/contracts",
}

// BuildCatalog derives the MCP tool surface from the live chi route table, so it
// stays exactly 1:1 with the real API and never drifts. Superadmin and public
// endpoints are filtered out.
func BuildCatalog(router chi.Routes) ([]ToolSpec, error) {
	tools := make([]ToolSpec, 0)
	seen := make(map[string]bool)

	walkErr := chi.Walk(router, func(method string, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = normalizeRoute(route)
		if !includeRoute(method, route) {
			return nil
		}

		name := toolName(method, route)
		if seen[name] {
			return nil
		}
		seen[name] = true

		tools = append(tools, ToolSpec{
			Name:         name,
			Description:  method + " " + route,
			Method:       method,
			PathTemplate: route,
			PathParams:   pathParams(route),
			Meta:         annotationFor(method, route),
		})
		return nil
	})

	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, walkErr
}

func includeRoute(method string, route string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	if !strings.HasPrefix(route, apiPrefix) {
		return false
	}
	if strings.Contains(route, "*") {
		return false
	}
	for _, suffix := range excludedSuffixes {
		if strings.HasSuffix(route, suffix) {
			return false
		}
	}
	for _, fragment := range excludedContains {
		if strings.Contains(route, fragment) {
			return false
		}
	}
	return true
}

// normalizeRoute strips chi regex constraints inside path params and any
// trailing slash so the template substitutes and names cleanly.
func normalizeRoute(route string) string {
	cleaned := stripParamConstraints(route)
	if len(cleaned) > len(apiPrefix) {
		cleaned = strings.TrimSuffix(cleaned, "/")
	}
	return cleaned
}

func stripParamConstraints(route string) string {
	var builder strings.Builder
	inParam := false
	skipping := false
	for _, char := range route {
		switch {
		case char == '{':
			inParam = true
			skipping = false
			builder.WriteRune(char)
		case char == '}':
			inParam = false
			skipping = false
			builder.WriteRune(char)
		case inParam && char == ':':
			skipping = true
		case inParam && skipping:
			// drop the regex constraint characters
		default:
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func toolName(method string, route string) string {
	trimmed := strings.TrimPrefix(route, apiPrefix)

	var builder strings.Builder
	builder.WriteString(strings.ToLower(method))
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" {
			continue
		}
		segment = strings.Trim(segment, "{}")
		builder.WriteString("_")
		builder.WriteString(sanitizeSegment(segment))
	}
	return builder.String()
}

func sanitizeSegment(segment string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(segment) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		} else {
			builder.WriteRune('_')
		}
	}
	return builder.String()
}

func pathParams(route string) []string {
	params := make([]string, 0)
	for _, segment := range strings.Split(route, "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			params = append(params, strings.Trim(segment, "{}"))
		}
	}
	return params
}
