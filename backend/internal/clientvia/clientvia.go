// Package clientvia marks API requests that arrive through the MCP tool
// surface, where an AI client acts for the user. The MCP server sets the
// marker on every request it builds; handlers use it to apply stricter rules
// (document e-mail). It can only narrow what a request may do, so a client
// that sets it on its own REST calls gains nothing.
package clientvia

import (
	"net/http"
	"strings"
)

const (
	Header = "X-Kantor-Via"
	MCP    = "mcp"
)

// IsMCP reports whether the request was built by the MCP server.
func IsMCP(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get(Header)), MCP)
}
