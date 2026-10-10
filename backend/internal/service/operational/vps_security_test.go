package operational

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
)

func TestValidateCheckTarget(t *testing.T) {
	tests := []struct {
		name      string
		checkType string
		target    string
		wantErr   bool
	}{
		{
			name:      "http accepts public hostname",
			checkType: "http",
			target:    "http://example.com/healthz",
		},
		{
			name:      "http rejects scheme mismatch",
			checkType: "http",
			target:    "https://example.com",
			wantErr:   true,
		},
		{
			name:      "https rejects localhost",
			checkType: "https",
			target:    "https://localhost",
			wantErr:   true,
		},
		{
			name:      "https rejects loopback literal",
			checkType: "https",
			target:    "https://127.0.0.1",
			wantErr:   true,
		},
		{
			name:      "tcp rejects private literal",
			checkType: "tcp",
			target:    "10.1.2.3:443",
			wantErr:   true,
		},
		{
			name:      "tcp accepts public literal",
			checkType: "tcp",
			target:    "8.8.8.8:53",
		},
		{
			name:      "tcp rejects non-numeric port",
			checkType: "tcp",
			target:    "example.com:https",
			wantErr:   true,
		},
		{
			name:      "icmp rejects localhost",
			checkType: "icmp",
			target:    "localhost",
			wantErr:   true,
		},
		{
			name:      "icmp accepts hostname",
			checkType: "icmp",
			target:    "example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCheckTarget(tc.checkType, tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil, got error: %v", err)
			}
		})
	}
}

func TestIsBlockedProbeIP(t *testing.T) {
	tests := []struct {
		addr    string
		blocked bool
	}{
		{addr: "127.0.0.1", blocked: true},
		{addr: "10.0.0.1", blocked: true},
		{addr: "169.254.1.1", blocked: true},
		{addr: "100.64.1.1", blocked: true},
		{addr: "198.18.0.1", blocked: true},
		{addr: "8.8.8.8", blocked: false},
		{addr: "1.1.1.1", blocked: false},
	}

	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tc.addr)
			if got := isBlockedProbeIP(addr); got != tc.blocked {
				t.Fatalf("isBlockedProbeIP(%s) = %v, want %v", tc.addr, got, tc.blocked)
			}
		})
	}
}

func TestVPSProbeClientRejectsRedirectToPrivateHost(t *testing.T) {
	var innerHits atomic.Int64
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer inner.Close()

	outer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, inner.URL, http.StatusFound)
	}))
	defer outer.Close()

	// The constructor's client is the real probe boundary: probeHTTP validates
	// only the initial host, so every hop the client itself dials must be
	// re-validated by the client's redirect policy.
	svc := NewVPSMonitorService(nil, nil, nil, nil)
	if svc.httpClient == nil {
		t.Fatal("probe http client is nil")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, outer.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := svc.httpClient.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("expected redirect to loopback to be rejected, got status %d and %d hits on inner server", resp.StatusCode, innerHits.Load())
	}
	if hits := innerHits.Load(); hits != 0 {
		t.Fatalf("inner (loopback) server was reached %d times through redirect", hits)
	}
}

func TestVPSProbeClientRedirectPolicy(t *testing.T) {
	svc := NewVPSMonitorService(nil, nil, nil, nil)
	check := svc.httpClient.CheckRedirect
	if check == nil {
		t.Fatal("expected probe client to install a CheckRedirect policy, got nil")
	}

	via, err := http.NewRequest(http.MethodGet, "http://1.1.1.1/health", nil)
	if err != nil {
		t.Fatalf("build via request: %v", err)
	}

	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{name: "public literal redirect allowed", target: "http://1.1.1.1/health", wantErr: false},
		{name: "private literal redirect blocked", target: "http://10.0.0.1/admin", wantErr: true},
		{name: "link-local metadata redirect blocked", target: "http://169.254.169.254/latest/meta-data/", wantErr: true},
		{name: "loopback redirect blocked", target: "http://127.0.0.1:8080/", wantErr: true},
		{name: "localhost hostname redirect blocked", target: "http://localhost/", wantErr: true},
		{name: "carrier-grade NAT literal blocked", target: "http://100.64.0.1/", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next, err := http.NewRequest(http.MethodGet, tc.target, nil)
			if err != nil {
				t.Fatalf("build redirect request: %v", err)
			}
			err = check(next, []*http.Request{via})
			if tc.wantErr && err == nil {
				t.Fatalf("expected redirect to %s to be blocked, got nil error", tc.target)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected redirect to %s to be allowed, got error: %v", tc.target, err)
			}
		})
	}
}

func TestVPSProbeClientRedirectPolicyKeepsHopLimit(t *testing.T) {
	svc := NewVPSMonitorService(nil, nil, nil, nil)
	check := svc.httpClient.CheckRedirect
	if check == nil {
		t.Fatal("expected probe client to install a CheckRedirect policy, got nil")
	}

	req, err := http.NewRequest(http.MethodGet, "http://1.1.1.1/health", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = req
	}

	if err := check(req, via); err == nil {
		t.Fatal("expected 11th redirect hop to be rejected like the default policy")
	}
}
