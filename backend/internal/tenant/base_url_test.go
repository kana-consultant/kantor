package tenant

import (
	"context"
	"errors"
	"testing"
)

type stubDomainRepository struct {
	domain string
	err    error
}

func (s stubDomainRepository) GetTenantPrimaryDomain(context.Context) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.domain, nil
}

func TestBaseURLFromDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		domain string
		want   string
	}{
		{name: "public domain uses https", domain: "kantor.example.com", want: "https://kantor.example.com"},
		{name: "localhost uses http", domain: "localhost:3000", want: "http://localhost:3000"},
		{name: "local domain uses http", domain: "tenantb.local:3000", want: "http://tenantb.local:3000"},
		{name: "prebuilt url preserved", domain: "https://kantor.example.com", want: "https://kantor.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := BaseURLFromDomain(tt.domain); got != tt.want {
				t.Fatalf("BaseURLFromDomain(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

func TestResolveBaseURL(t *testing.T) {
	t.Parallel()

	t.Run("prefers tenant primary domain", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveBaseURL(context.Background(), stubDomainRepository{domain: "kantor.perfect10.bot"}, "http://legacy.local:3000")
		if err != nil {
			t.Fatalf("ResolveBaseURL returned error: %v", err)
		}
		if got != "https://kantor.perfect10.bot" {
			t.Fatalf("ResolveBaseURL() = %q, want %q", got, "https://kantor.perfect10.bot")
		}
	})

	t.Run("falls back when no domain is configured", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveBaseURL(context.Background(), stubDomainRepository{}, "http://legacy.local:3000/")
		if err != nil {
			t.Fatalf("ResolveBaseURL returned error: %v", err)
		}
		if got != "http://legacy.local:3000" {
			t.Fatalf("ResolveBaseURL() = %q, want %q", got, "http://legacy.local:3000")
		}
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("boom")
		if _, err := ResolveBaseURL(context.Background(), stubDomainRepository{err: wantErr}, "http://legacy.local:3000"); !errors.Is(err, wantErr) {
			t.Fatalf("ResolveBaseURL error = %v, want %v", err, wantErr)
		}
	})
}
