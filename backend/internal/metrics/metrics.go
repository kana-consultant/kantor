// Package metrics exposes Prometheus instrumentation for the HTTP layer.
//
// The server registers a tiny default set: request count, request duration
// histogram, and an in-flight gauge — labelled by method, route template,
// and response status. Service-layer counters can be added next to the
// business logic that produces them; this package owns only the HTTP wiring
// so router setup is the single place to ship "metrics on / off".
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds the prometheus collectors we own. A dedicated registry keeps
// our metrics out of the global default — callers can scrape just /metrics
// without inheriting whatever the rest of the process registers.
type Registry struct {
	registry        *prometheus.Registry
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	inflight        prometheus.Gauge
}

// NewRegistry builds and registers the default HTTP collectors.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "kantor",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests, labelled by method, route template, and status class.",
		},
		[]string{"method", "route", "status"},
	)
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "kantor",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration distribution.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)
	inflight := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "kantor",
			Subsystem: "http",
			Name:      "inflight_requests",
			Help:      "Number of HTTP requests currently being served.",
		},
	)

	reg.MustRegister(
		requestsTotal,
		requestDuration,
		inflight,
		// Standard process collectors — gives us memory, goroutines, gc, fds for free.
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
	)

	return &Registry{
		registry:        reg,
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
		inflight:        inflight,
	}
}

// Handler returns the http.Handler that serves /metrics in the Prometheus
// text exposition format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{
		Registry: r.registry,
	})
}

// Middleware records request count, duration, and concurrency for every HTTP
// handler. It uses chi's RouteContext so metrics labels carry the route
// template (e.g. /api/v1/admin/users/{userID}) instead of the raw path,
// keeping label cardinality bounded.
func (r *Registry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.inflight.Inc()
		defer r.inflight.Dec()

		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, req)

		route := chiRoutePattern(req)
		method := req.Method
		status := strconv.Itoa(recorder.status)
		duration := time.Since(start).Seconds()

		r.requestsTotal.WithLabelValues(method, route, status).Inc()
		r.requestDuration.WithLabelValues(method, route).Observe(duration)
	})
}

// chiRoutePattern returns the matched route template, e.g.
// "/api/v1/hris/employees/{employeeID}", or "unmatched" when chi did not
// find a handler. Using the template keeps label cardinality bounded.
func chiRoutePattern(req *http.Request) string {
	if rctx := chi.RouteContext(req.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unmatched"
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}
