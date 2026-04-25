package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestSetup_NoExporterReturnsNoOpShutdown(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Setup must return a non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	// Even without an exporter, the tracer must still produce a usable Tracer
	// so downstream code is safe to call otel.Tracer(...).Start().
	tracer := otel.Tracer("kantor.test")
	if tracer == nil {
		t.Fatal("otel.Tracer returned nil")
	}
	_, span := tracer.Start(context.Background(), "noop-span")
	span.End()
}

func TestSetup_InstallsW3CPropagator(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer shutdown(context.Background()) //nolint:errcheck

	prop := otel.GetTextMapPropagator()
	if prop == nil {
		t.Fatal("expected propagator to be set")
	}

	// Round-trip a traceparent through the registered propagator: extract,
	// inject. Successful round-trip means TraceContext is in the chain.
	carrier := propagation.MapCarrier{}
	carrier.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	ctx := prop.Extract(context.Background(), carrier)

	out := propagation.MapCarrier{}
	prop.Inject(ctx, out)
	if out.Get("traceparent") == "" {
		t.Fatal("propagator did not preserve traceparent across Extract/Inject")
	}
}
