package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/clientvia"
)

// Executor runs a built HTTP request against the Kantor API and returns the
// status and raw body. The in-process variant re-enters the chi router so the
// full tenant + auth + RBAC chain applies; the remote variant calls a running
// server over the network.
type Executor interface {
	Execute(req *http.Request) (int, []byte, error)
}

type InProcessExecutor struct {
	Handler func() http.Handler
}

func (e InProcessExecutor) Execute(req *http.Request) (int, []byte, error) {
	// Re-enter the router with a fresh context: the incoming /mcp request's
	// context still carries chi's routing state, which would otherwise be
	// reused and mis-route this sub-request.
	req = req.WithContext(context.Background())
	recorder := httptest.NewRecorder()
	e.Handler().ServeHTTP(recorder, req)
	return recorder.Code, recorder.Body.Bytes(), nil
}

type RemoteExecutor struct {
	Client *http.Client
}

func (e RemoteExecutor) Execute(req *http.Request) (int, []byte, error) {
	resp, err := e.Client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, data, nil
}

func (t ToolSpec) buildRequest(ctx context.Context, baseURL string, args map[string]json.RawMessage, authHeader string, host string) (*http.Request, error) {
	path := t.PathTemplate
	for _, param := range t.PathParams {
		raw, ok := args[param]
		if !ok {
			return nil, fmt.Errorf("missing path parameter %q", param)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("path parameter %q must be a string", param)
		}
		path = strings.ReplaceAll(path, "{"+param+"}", url.PathEscape(value))
	}

	query := url.Values{}
	if raw, ok := args["query"]; ok && len(raw) > 0 {
		var parsed map[string]interface{}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("query must be an object")
		}
		for key, value := range parsed {
			addQueryValue(query, key, value)
		}
	}

	if t.Meta != nil && t.Meta.Paginated {
		clampPerPage(query, t.Meta.PerPageMax)
	}

	target := baseURL + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}

	var body io.Reader
	if t.hasBody() {
		if raw, ok := args["body"]; ok && len(raw) > 0 {
			body = bytes.NewReader(raw)
		} else {
			body = bytes.NewReader([]byte("{}"))
		}
	}

	req, err := http.NewRequestWithContext(ctx, t.Method, target, body)
	if err != nil {
		return nil, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if host != "" {
		req.Host = host
	}
	if t.hasBody() {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	// Handlers apply stricter rules to calls an AI client makes for the user
	// (document e-mail goes only to a login address the human confirmed).
	req.Header.Set(clientvia.Header, clientvia.MCP)
	return req, nil
}

// addQueryValue encodes a JSON query value into url.Values. JSON numbers decode
// as float64, so integers must render without scientific notation/decimals, and
// arrays become repeated keys (?k=a&k=b). fmt.Sprintf("%v") mangled all of these
// (e.g. a large per_page -> "1e+06", an array -> "[a b]"), corrupting filters.
// mcpPerPageCap is the hard upper bound MCP allows for per_page, so an AI client
// cannot pull an entire table in one call (some endpoints have no server-side cap).
const mcpPerPageCap = 100

// clampPerPage limits an AI-supplied per_page, using the endpoint's own max when
// known, else the global MCP cap.
func clampPerPage(query url.Values, endpointMax int) {
	limit := mcpPerPageCap
	if endpointMax > 0 && endpointMax < limit {
		limit = endpointMax
	}
	raw := query.Get("per_page")
	if raw == "" {
		return
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return
	}
	if n > limit {
		query.Set("per_page", strconv.Itoa(limit))
	}
}

func addQueryValue(query url.Values, key string, value interface{}) {
	switch v := value.(type) {
	case nil:
		return
	case string:
		query.Add(key, v)
	case bool:
		query.Add(key, strconv.FormatBool(v))
	case float64:
		if !math.IsInf(v, 0) && !math.IsNaN(v) && v == math.Trunc(v) {
			query.Add(key, strconv.FormatInt(int64(v), 10))
		} else {
			query.Add(key, strconv.FormatFloat(v, 'f', -1, 64))
		}
	case []interface{}:
		for _, item := range v {
			addQueryValue(query, key, item)
		}
	default:
		query.Add(key, fmt.Sprintf("%v", v))
	}
}
