package mcp

import (
	"fmt"
	"net/http"
)

type ToolSpec struct {
	Name         string
	Description  string
	Method       string
	PathTemplate string
	PathParams   []string
	// Meta enriches the query schema; nil endpoints keep the generic one.
	Meta *EndpointMeta
}

// QueryParam describes one real query-string parameter of an endpoint.
type QueryParam struct {
	Name        string
	Type        string // "string", "integer", "boolean"
	Description string
	Enum        []string
}

// EndpointMeta is the curated annotation for an endpoint.
type EndpointMeta struct {
	Description    string
	Query          []QueryParam
	Paginated      bool
	PerPageDefault int
	PerPageMax     int // 0 = no enforced cap
	// Body documents the JSON request body; the schema stays a free-form
	// object, so this text is what tells an AI client which fields exist.
	Body string
	// Destructive marks a non-DELETE tool whose effect cannot be undone (an
	// e-mail goes out, a sent document is voided, a contract is closed).
	Destructive bool
	// OpenWorld marks a tool that reaches people outside the API (e-mail).
	OpenWorld bool
	// RequireConfirm makes the tool refuse to run unless the call carries
	// confirm=true, so an AI client has to stop and ask the human first.
	RequireConfirm bool
}

// confirmArg is the top-level tool argument RequireConfirm tools need. It is
// never forwarded to the API.
const confirmArg = "confirm"

const confirmArgDescription = "Set to true only after the human has seen exactly who will receive what and has explicitly approved this send in the conversation; the approved address(es) go in the body as expected_recipient(s). Never set it on your own initiative."

func (t ToolSpec) requiresConfirm() bool {
	return t.Meta != nil && t.Meta.RequireConfirm
}

func (t ToolSpec) hasBody() bool {
	return t.Method == http.MethodPost || t.Method == http.MethodPut || t.Method == http.MethodPatch
}

func (t ToolSpec) inputSchema() map[string]interface{} {
	properties := map[string]interface{}{}
	required := make([]string, 0, len(t.PathParams))

	for _, param := range t.PathParams {
		properties[param] = map[string]interface{}{
			"type":        "string",
			"description": "Path parameter " + param,
		}
		required = append(required, param)
	}

	properties["query"] = t.querySchema()

	if t.hasBody() {
		bodyDescription := "JSON request body for this endpoint."
		if t.Meta != nil && t.Meta.Body != "" {
			bodyDescription = "JSON request body: " + t.Meta.Body
		}
		properties["body"] = map[string]interface{}{
			"type":        "object",
			"description": bodyDescription,
		}
	}

	if t.requiresConfirm() {
		properties[confirmArg] = map[string]interface{}{
			"type":        "boolean",
			"description": confirmArgDescription,
		}
		required = append(required, confirmArg)
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// querySchema renders named filter + pagination properties when metadata exists,
// else a free-form query object.
func (t ToolSpec) querySchema() map[string]interface{} {
	if t.Meta == nil || (len(t.Meta.Query) == 0 && !t.Meta.Paginated) {
		return map[string]interface{}{
			"type":        "object",
			"description": "Optional query-string parameters as key/value pairs.",
		}
	}

	props := map[string]interface{}{}
	for _, param := range t.Meta.Query {
		prop := map[string]interface{}{"type": param.Type}
		if param.Description != "" {
			prop["description"] = param.Description
		}
		if len(param.Enum) > 0 {
			prop["enum"] = param.Enum
		}
		props[param.Name] = prop
	}

	if t.Meta.Paginated {
		props["page"] = map[string]interface{}{
			"type":        "integer",
			"minimum":     1,
			"description": "1-based page number (default 1).",
		}
		perPage := map[string]interface{}{"type": "integer", "minimum": 1}
		desc := fmt.Sprintf("Items per page (default %d", t.Meta.PerPageDefault)
		if t.Meta.PerPageMax > 0 {
			desc += fmt.Sprintf(", max %d", t.Meta.PerPageMax)
			perPage["maximum"] = t.Meta.PerPageMax
		}
		desc += "). The response is paginated — page through every page to retrieve all data."
		perPage["description"] = desc
		props["per_page"] = perPage
	}

	return map[string]interface{}{
		"type":        "object",
		"description": "Query-string filters and pagination.",
		"properties":  props,
	}
}

func (t ToolSpec) descriptor() map[string]interface{} {
	description := t.Description
	if t.Meta != nil && t.Meta.Description != "" {
		description = t.Meta.Description + " (" + t.Method + " " + t.PathTemplate + ")"
	}
	annotations := map[string]interface{}{
		"readOnlyHint":    t.Method == http.MethodGet,
		"destructiveHint": t.Method == http.MethodDelete || (t.Meta != nil && t.Meta.Destructive),
		"idempotentHint":  t.Method == http.MethodGet || t.Method == http.MethodPut || t.Method == http.MethodDelete,
	}
	if t.Meta != nil && t.Meta.OpenWorld {
		annotations["openWorldHint"] = true
	}
	return map[string]interface{}{
		"name":        t.Name,
		"description": description,
		"inputSchema": t.inputSchema(),
		"annotations": annotations,
	}
}
