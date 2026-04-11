package tenant

import (
	"context"
	"net"
	"strings"
)

type DomainRepository interface {
	GetTenantPrimaryDomain(ctx context.Context) (string, error)
}

func ResolveBaseURL(ctx context.Context, repo DomainRepository, fallback string) (string, error) {
	if repo != nil {
		domain, err := repo.GetTenantPrimaryDomain(ctx)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(domain) != "" {
			return BaseURLFromDomain(domain), nil
		}
	}

	return strings.TrimRight(strings.TrimSpace(fallback), "/"), nil
}

func BaseURLFromDomain(domain string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(domain), "/")
	if trimmed == "" {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}

	host := trimmed
	hostWithoutPort := host
	if parsedHost, parsedPort, err := net.SplitHostPort(host); err == nil && parsedHost != "" && parsedPort != "" {
		hostWithoutPort = parsedHost
	}

	switch {
	case hostWithoutPort == "localhost",
		hostWithoutPort == "127.0.0.1",
		hostWithoutPort == "::1",
		strings.HasSuffix(hostWithoutPort, ".local"):
		return "http://" + host
	default:
		return "https://" + host
	}
}
