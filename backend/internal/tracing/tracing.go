// Package tracing wires OpenTelemetry into the backend.
//
// Behaviour is controlled by env vars:
//   - OTEL_EXPORTER_OTLP_ENDPOINT="" (default) -> no-op tracer. Code that
//     calls `otel.Tracer(...).Start()` still works, traces are simply
//     dropped, so adding spans to business code is always safe.
//   - OTEL_EXPORTER_OTLP_ENDPOINT="otel-collector:4317" -> OTLP/gRPC
//     exporter, batch span processor, configurable via the standard
//     OTEL_* environment variables.
package tracing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const serviceName = "kantor-backend"

// Setup installs a global TracerProvider and returns a shutdown function the
// caller MUST run before exit so any buffered spans get flushed.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is empty the function configures a no-op
// provider. otel.Tracer() callers still work — they simply produce no
// telemetry — so business code can sprinkle spans without worrying about
// whether tracing is actually wired up in the current environment.
func Setup(ctx context.Context, version string) (shutdown func(context.Context) error, err error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))

	// Always install the W3C TraceContext + Baggage propagators so incoming
	// traceparent headers populate the request context even when this
	// instance has no exporter wired up.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("build trace resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
