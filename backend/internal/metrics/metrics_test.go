package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMiddlewareEmitsRequestMetrics(t *testing.T) {
	reg := NewRegistry()

	router := chi.NewRouter()
	router.Use(reg.Middleware)
	router.Get("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users/42")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", resp.StatusCode)
	}

	metricsResp, err := http.Get(srv.URL + "/__metrics_should_404") // sanity that /metrics route is owned by the registry, not chi
	if err == nil {
		metricsResp.Body.Close()
	}

	body := scrape(t, reg)
	// chi route template is preserved as a label, not the raw path.
	if !strings.Contains(body, `kantor_http_requests_total{method="GET",route="/users/{id}",status="418"} 1`) {
		t.Fatalf("missing per-request counter line; scrape:\n%s", body)
	}
	if !strings.Contains(body, `kantor_http_request_duration_seconds_count{method="GET",route="/users/{id}"} 1`) {
		t.Fatalf("missing duration histogram count; scrape:\n%s", body)
	}
}

func TestMiddlewareTagsUnmatchedRoutes(t *testing.T) {
	// Wrap the chi router so middleware runs even on NotFound — chi attaches
	// our middleware to the matched-route chain, not to its synthetic 404.
	reg := NewRegistry()
	router := chi.NewRouter()
	router.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	wrapped := reg.Middleware(router)
	srv := httptest.NewServer(wrapped)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	body := scrape(t, reg)
	if !strings.Contains(body, `kantor_http_requests_total{method="GET",route="unmatched",status="404"} 1`) {
		t.Fatalf("expected unmatched-route counter line; scrape:\n%s", body)
	}
}

func scrape(t *testing.T, reg *Registry) string {
	t.Helper()
	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("scrape body: %v", err)
	}
	return string(body)
}
