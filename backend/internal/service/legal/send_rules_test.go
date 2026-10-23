package legal

import (
	"errors"
	"testing"
)

func TestCheckSendRecipient(t *testing.T) {
	cases := []struct {
		name      string
		party     string
		recipient string
		expected  string
		viaMCP    bool
		want      error
	}{
		{"web: any address", "mitra@example.com", "lain@example.com", "", false, nil},
		{"web: expected must match", "mitra@example.com", "lain@example.com", "mitra@example.com", false, ErrRecipientMismatch},
		{"mcp: party address approved", "Mitra@Example.com", "mitra@example.com", "MITRA@example.com", true, nil},
		{"mcp: expected required", "mitra@example.com", "mitra@example.com", "", true, ErrExpectedRecipientRequired},
		{"mcp: other address refused", "mitra@example.com", "attacker@example.com", "attacker@example.com", true, ErrMCPRecipientRestricted},
		{"mcp: party without e-mail", "", "mitra@example.com", "mitra@example.com", true, ErrMCPRecipientRestricted},
		{"mcp: mismatch wins", "mitra@example.com", "mitra@example.com", "lain@example.com", true, ErrRecipientMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkSendRecipient(tc.party, tc.recipient, tc.expected, tc.viaMCP); !errors.Is(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
