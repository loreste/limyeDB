// Package observability provides tracing support using OpenTelemetry.
package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// TracerConfig holds configuration for the tracer.
type TracerConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Endpoint       string
	SampleRate     float64
	Enabled        bool
}

// DefaultTracerConfig returns default tracer configuration.
func DefaultTracerConfig() *TracerConfig {
	return &TracerConfig{
		ServiceName:    "limyedb",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Endpoint:       "http://localhost:4318",
		SampleRate:     0.1,
		Enabled:        false,
	}
}

// Tracer wraps OpenTelemetry tracer with LimyeDB-specific functionality.
type Tracer struct {
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
	config   *TracerConfig
}

// NewTracer creates a new tracer with the given configuration.
func NewTracer(config *TracerConfig) (*Tracer, error) {
	if !config.Enabled {
		return &Tracer{
			tracer: otel.Tracer(config.ServiceName),
			config: config,
		}, nil
	}

	ctx := context.Background()

	// Create OTLP HTTP exporter
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint(config.Endpoint),
		otlptracehttp.WithInsecure(),
	)

	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			attribute.String("environment", config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler
	var sampler sdktrace.Sampler
	if config.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if config.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(config.SampleRate)
	}

	// Create trace provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global provider and propagator
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{
		tracer:   provider.Tracer(config.ServiceName),
		provider: provider,
		config:   config,
	}, nil
}

// Shutdown gracefully shuts down the tracer.
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t.provider != nil {
		return t.provider.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span.
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the span from context.
func (t *Tracer) SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceSearch traces a search operation.
func (t *Tracer) TraceSearch(ctx context.Context, collection string, k int) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "search",
		trace.WithAttributes(
			attribute.String("collection", collection),
			attribute.Int("k", k),
		),
	)
}

// TraceInsert traces an insert operation.
func (t *Tracer) TraceInsert(ctx context.Context, collection string, count int) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "insert",
		trace.WithAttributes(
			attribute.String("collection", collection),
			attribute.Int("count", count),
		),
	)
}

// TraceHNSWSearch traces an HNSW search.
func (t *Tracer) TraceHNSWSearch(ctx context.Context, collection string, ef int) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "hnsw.search",
		trace.WithAttributes(
			attribute.String("collection", collection),
			attribute.Int("ef_search", ef),
		),
	)
}

// TraceRaftOperation traces a Raft operation.
func (t *Tracer) TraceRaftOperation(ctx context.Context, op string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "raft."+op,
		trace.WithAttributes(
			attribute.String("operation", op),
		),
	)
}

// AddSearchResultAttributes adds search result attributes to span.
func AddSearchResultAttributes(span trace.Span, resultCount int, visitedNodes int, durationMs float64) {
	span.SetAttributes(
		attribute.Int("result_count", resultCount),
		attribute.Int("visited_nodes", visitedNodes),
		attribute.Float64("duration_ms", durationMs),
	)
}

// AddErrorAttribute adds error information to span.
func AddErrorAttribute(span trace.Span, err error) {
	span.SetAttributes(attribute.String("error", err.Error()))
	span.RecordError(err)
}
