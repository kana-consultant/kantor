package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPResolver_UntrustedRemoteIgnoresForwardedHeader(t *testing.T) {
	resolver, err := newClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:44321"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")

	if got := resolver.Resolve(req); got != "203.0.113.10" {
		t.Fatalf("expected direct client ip, got %q", got)
	}
}

func TestClientIPResolver_TrustedRemoteUsesFirstUntrustedFromRight(t *testing.T) {
	resolver, err := newClientIPResolver([]string{"10.0.0.0/8", "192.168.0.0/16"})
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.5:44321"
	req.Header.Set("X-Forwarded-For", "198.51.100.99, 192.168.0.20")

	if got := resolver.Resolve(req); got != "198.51.100.99" {
		t.Fatalf("expected forwarded client ip, got %q", got)
	}
}

func TestClientIPResolver_AllForwardedTrustedFallsBackToRemote(t *testing.T) {
	resolver, err := newClientIPResolver([]string{"10.0.0.0/8", "192.168.0.0/16"})
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.5:44321"
	req.Header.Set("X-Forwarded-For", "10.1.2.3, 192.168.0.20")

	if got := resolver.Resolve(req); got != "10.0.0.5" {
		t.Fatalf("expected remote ip fallback, got %q", got)
	}
}
