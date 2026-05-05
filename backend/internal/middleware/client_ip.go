package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

type clientIPContextKey struct{}

var defaultTrustedProxyCIDRs = []string{
	"127.0.0.1/32",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"fc00::/7",
	"fe80::/10",
}

type clientIPResolver struct {
	trustedProxyNets []*net.IPNet
}

func NewClientIPMiddleware(trustedProxyCIDRs []string) (func(http.Handler) http.Handler, error) {
	resolver, err := newClientIPResolver(trustedProxyCIDRs)
	if err != nil {
		return nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := resolver.Resolve(r)
			ctx := context.WithValue(r.Context(), clientIPContextKey{}, clientIP)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}

func ClientIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}

	if ip, ok := r.Context().Value(clientIPContextKey{}).(string); ok && strings.TrimSpace(ip) != "" {
		return ip
	}

	return parseRemoteIP(r.RemoteAddr)
}

func newClientIPResolver(trustedProxyCIDRs []string) (*clientIPResolver, error) {
	cidrs := trustedProxyCIDRs
	if len(cidrs) == 0 {
		cidrs = defaultTrustedProxyCIDRs
	}

	trustedProxyNets := make([]*net.IPNet, 0, len(cidrs))
	for _, item := range cidrs {
		cidr := strings.TrimSpace(item)
		if cidr == "" {
			continue
		}

		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parse trusted proxy cidr %q: %w", cidr, err)
		}
		trustedProxyNets = append(trustedProxyNets, network)
	}

	return &clientIPResolver{trustedProxyNets: trustedProxyNets}, nil
}

func (r *clientIPResolver) Resolve(req *http.Request) string {
	remoteIP := parseRemoteIP(req.RemoteAddr)
	if remoteIP == "" {
		return ""
	}

	parsedRemote := net.ParseIP(remoteIP)
	if parsedRemote == nil || !r.isTrustedProxy(parsedRemote) {
		return remoteIP
	}

	candidates := forwardedCandidates(req)
	for idx := len(candidates) - 1; idx >= 0; idx-- {
		candidateIP := net.ParseIP(candidates[idx])
		if candidateIP == nil {
			continue
		}
		if !r.isTrustedProxy(candidateIP) {
			return candidateIP.String()
		}
	}

	return remoteIP
}

func (r *clientIPResolver) isTrustedProxy(ip net.IP) bool {
	for _, network := range r.trustedProxyNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func forwardedCandidates(req *http.Request) []string {
	values := req.Header.Values("X-Forwarded-For")
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, value := range values {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			if ip := strings.TrimSpace(part); ip != "" {
				out = append(out, ip)
			}
		}
	}

	return out
}

func parseRemoteIP(remoteAddr string) string {
	trimmed := strings.TrimSpace(remoteAddr)
	if trimmed == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		return host
	}

	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
	}

	return trimmed
}
