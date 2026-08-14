// Package observability provides OpenTelemetry tracing initialization and HTTP
// span instrumentation.
package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// InitTracer sets up the global tracer provider, exporting to an OTLP HTTP
// endpoint. An empty endpoint leaves tracing disabled (no-op). It returns a
// shutdown function.
func InitTracer(ctx context.Context, endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			"https://opentelemetry.io/schemas/1.24.0",
			attribute.String("service.name", "simulator"),
		)),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Middleware starts a span for each HTTP request.
func Middleware(next http.Handler) http.Handler {
	tracer := otel.Tracer("simulator")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), r.Method+" "+r.URL.Path)
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
