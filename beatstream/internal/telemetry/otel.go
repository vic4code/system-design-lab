// Package telemetry sets up OpenTelemetry tracing — the third pillar of observability.
//
// Three pillars:
//   - Logs:    zap JSON (internal/logger) → CloudWatch Logs
//   - Metrics: Prometheus (internal/metrics) → CloudWatch Metrics
//   - Traces:  OpenTelemetry → Jaeger (local) / AWS X-Ray (production)
//
// All three are correlated via trace_id / request_id.
//
// Local:      OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318
// Production: OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 (ADOT sidecar → X-Ray)
//
// AWS X-Ray compatibility:
//   Use the AWS Distro for OpenTelemetry (ADOT) collector as an ECS sidecar.
//   The collector receives OTLP from the app and forwards to X-Ray.
//   No X-Ray SDK needed — standard OTel SDK works transparently.
package telemetry

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init initialises the global OpenTelemetry TracerProvider.
//
// OTEL_EXPORTER_OTLP_ENDPOINT: OTLP HTTP endpoint (default: http://localhost:4318)
//   Local dev:   http://jaeger:4318  (Jaeger all-in-one docker container)
//   Production:  http://localhost:4318 (ADOT sidecar in same ECS task)
//
// OTEL_SERVICE_NAME: overrides the default "beatstream-api"
//
// Returns a shutdown function — call in main() defer.
func Init(ctx context.Context) (shutdown func(context.Context) error, err error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:4318"
	}

	// OTLP over HTTP exporter (works with Jaeger, Tempo, ADOT, Honeycomb, etc.)
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(), // TLS handled at the network layer in production
		otlptracehttp.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, err
	}

	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "beatstream-api"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(os.Getenv("APP_VERSION")),
			attribute.String("deployment.environment", os.Getenv("APP_ENV")),
		),
		resource.WithOS(),
		resource.WithProcess(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		// Sample 100% in dev; in production use ParentBased(TraceIDRatioBased(0.1))
		// to sample 10% — reduce storage costs while preserving full traces for errors
		sdktrace.WithSampler(samplerFromEnv()),
	)

	// Set globals so any code can call otel.Tracer() without passing tp around
	otel.SetTracerProvider(tp)

	// W3C TraceContext propagation — allows trace_id to flow across HTTP boundaries:
	// nginx → API → Kafka messages → worker
	// The traceparent header format: 00-{trace_id}-{span_id}-{flags}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// samplerFromEnv reads OTEL_TRACES_SAMPLER_ARG (0.0–1.0) for production cost control.
// Default: sample everything (AlwaysSample) — appropriate for local dev.
func samplerFromEnv() sdktrace.Sampler {
	if os.Getenv("APP_ENV") == "production" {
		// Sample 10% of root spans in production; always sample if parent was sampled
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))
	}
	return sdktrace.AlwaysSample()
}
