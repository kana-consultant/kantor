package mcp

import (
	"context"
	"encoding/json"
)

const serverName = "kantor-mcp"

type Server struct {
	tools     []ToolSpec
	byName    map[string]ToolSpec
	executor  Executor
	baseURL   string
	version   string
	authorize func(token string) bool
}

// SetAuthorizer registers a bearer-token validator. When set, the HTTP
// transport replies 401 + WWW-Authenticate (triggering OAuth discovery) for
// requests without a valid token.
func (s *Server) SetAuthorizer(fn func(token string) bool) {
	s.authorize = fn
}

func NewServer(tools []ToolSpec, executor Executor, baseURL string, version string) *Server {
	byName := make(map[string]ToolSpec, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	return &Server{
		tools:    tools,
		byName:   byName,
		executor: executor,
		baseURL:  baseURL,
		version:  version,
	}
}

// Handle processes a single JSON-RPC message. It returns the encoded response,
// or isNotification=true when the message is a notification that expects no
// reply.
func (s *Server) Handle(ctx context.Context, raw []byte, authHeader string, host string) (encoded []byte, isNotification bool) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return marshal(newError(nil, codeParseError, "parse error")), false
	}

	switch req.Method {
	case "initialize":
		return marshal(newResult(req.ID, s.initializeResult())), false
	case "ping":
		return marshal(newResult(req.ID, map[string]interface{}{})), false
	case "tools/list":
		return marshal(newResult(req.ID, map[string]interface{}{"tools": s.toolDescriptors()})), false
	case "tools/call":
		return marshal(s.callTool(ctx, req, authHeader, host)), false
	default:
		if len(req.ID) == 0 {
			return nil, true
		}
		return marshal(newError(req.ID, codeMethodMissing, "method not found: "+req.Method)), false
	}
}

func (s *Server) initializeResult() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{"listChanged": false},
		},
		"serverInfo": map[string]interface{}{
			"name":    serverName,
			"version": s.version,
		},
	}
}

func (s *Server) toolDescriptors() []map[string]interface{} {
	descriptors := make([]map[string]interface{}, 0, len(s.tools))
	for _, tool := range s.tools {
		descriptors = append(descriptors, tool.descriptor())
	}
	return descriptors
}

func (s *Server) callTool(ctx context.Context, req request, authHeader string, host string) response {
	var params struct {
		Name      string                     `json:"name"`
		Arguments map[string]json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newError(req.ID, codeInvalidParams, "invalid params")
	}

	tool, ok := s.byName[params.Name]
	if !ok {
		return newError(req.ID, codeInvalidParams, "unknown tool: "+params.Name)
	}

	if tool.requiresConfirm() && !confirmed(params.Arguments) {
		// A tool result (not a protocol error), so the model reads what to do.
		return newResult(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": confirmRequiredText},
			},
			"isError": true,
		})
	}

	httpReq, err := tool.buildRequest(ctx, s.baseURL, params.Arguments, authHeader, host)
	if err != nil {
		return newError(req.ID, codeInvalidParams, err.Error())
	}

	status, body, err := s.executor.Execute(httpReq)
	if err != nil {
		return newError(req.ID, codeInternalError, "tool execution failed: "+err.Error())
	}

	return newResult(req.ID, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": boundToolText(body)},
		},
		"isError": status >= 400,
	})
}

const confirmRequiredText = "Nothing was sent. This tool e-mails documents to employees and cannot be undone. " +
	"First show the human who will receive what (payslips: send-preview or recipient; contracts: recipient), " +
	"wait for their explicit approval in the conversation, then call the tool again with confirm=true " +
	"and the approved address(es) in the body as expected_recipient(s)."

// confirmed reports whether the call carries the literal JSON true for the
// confirm argument; "true", 1 and friends do not count.
func confirmed(args map[string]json.RawMessage) bool {
	raw, ok := args[confirmArg]
	if !ok {
		return false
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	flag, isBool := value.(bool)
	return isBool && flag
}

// maxToolResponseBytes caps a single tool result so one over-broad query cannot
// blow up the AI client's context window (which surfaces as upstream 529
// "Overloaded" errors). Clients should paginate / filter instead.
const maxToolResponseBytes = 100 * 1024

func boundToolText(body []byte) string {
	if len(body) <= maxToolResponseBytes {
		return string(body)
	}
	return string(body[:maxToolResponseBytes]) +
		"\n\n[response truncated at 100KB — narrow it with query filters or a smaller per_page, then page through the rest]"
}
