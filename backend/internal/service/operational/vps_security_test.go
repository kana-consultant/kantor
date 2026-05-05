package operational

import (
	"net/netip"
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
