package auth

import "testing"

func ptr(s string) *string { return &s }

func TestRefreshUserAgentMatches_EmptyStoredAcceptsAnything(t *testing.T) {
	if !refreshUserAgentMatches(nil, "Mozilla/5.0 ...") {
		t.Fatal("nil stored UA should be treated as a match (legacy rows)")
	}
	if !refreshUserAgentMatches(ptr(""), "anything") {
		t.Fatal("empty stored UA should be treated as a match")
	}
}

func TestRefreshUserAgentMatches_SameBrowserWithMinorUpdate(t *testing.T) {
	old := ptr("Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/605.1.15")
	updated := "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Chrome/125.0.4321.99 Safari/605.1.15"
	if !refreshUserAgentMatches(old, updated) {
		t.Fatal("minor browser version bump must not invalidate the refresh token")
	}
}

func TestRefreshUserAgentMatches_DifferentBrowserRejected(t *testing.T) {
	stored := ptr("Mozilla/5.0 (Windows NT 10.0) Gecko/20100101 Firefox/123.0")
	other := "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/124.0.0.0"
	if refreshUserAgentMatches(stored, other) {
		t.Fatal("a different browser must not be treated as the same client")
	}
}

func TestRefreshUserAgentMatches_CurlReplayRejected(t *testing.T) {
	stored := ptr("Mozilla/5.0 (Macintosh) Chrome/124.0.0.0 Safari/605.1.15")
	curl := "curl/8.4.0"
	if refreshUserAgentMatches(stored, curl) {
		t.Fatal("curl replay of a browser-issued refresh token must be rejected")
	}
}

func TestUserAgentFingerprint_StableForSameVendor(t *testing.T) {
	a := userAgentFingerprint("Mozilla/5.0 ... Chrome/124 ... Safari/605")
	b := userAgentFingerprint("Mozilla/5.0 ... Chrome/126 ... Safari/605")
	if a != b {
		t.Fatalf("fingerprints should match across minor Chrome versions, got %q vs %q", a, b)
	}
}

func TestUserAgentFingerprint_DistinguishesVendors(t *testing.T) {
	a := userAgentFingerprint("Mozilla/5.0 ... Firefox/123")
	b := userAgentFingerprint("Mozilla/5.0 ... Chrome/124 Safari/605")
	if a == b {
		t.Fatalf("Firefox and Chrome must produce different fingerprints, both got %q", a)
	}
}
