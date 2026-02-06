package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type telemetryConfig struct {
	ServiceName  string
	Environment  string
	OtlpEndpoint string // host:port e.g. "otel-collector.observability.svc.cluster.local:4317"
}

func loadTelemetryConfig() telemetryConfig {
	// OTEL_EXPORTER_OTLP_ENDPOINT can be:
	// - "otel-collector.observability.svc.cluster.local:4317"  (recommended style)
	// - OR "http://..." (some setups). We'll normalize below.
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "otel-collector.observability.svc.cluster.local:4317"
	}

	// normalize: strip http(s):// if present
	endpoint = stripScheme(endpoint)

	return telemetryConfig{
		ServiceName:  getenv("APP_NAME", "service"),
		Environment:  getenv("ENVIRONMENT", "dev"),
		OtlpEndpoint: endpoint,
	}
}

func initTracer(ctx context.Context, cfg telemetryConfig) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(cfg.OtlpEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{MinConnectTimeout: 2 * time.Second}),
	)
	if err != nil {
		return nil, fmt.Errorf("otlp grpc dial: %w", err)
	}

	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))), // 100% for now; tune later
		sdktrace.WithBatcher(exp),
	)

	otel.SetTracerProvider(tp)

	// IMPORTANT: so trace context flows across services via HTTP headers
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp.Shutdown, nil
}

func stripScheme(s string) string {
	if len(s) >= 7 && s[:7] == "http://" {
		return s[7:]
	}
	if len(s) >= 8 && s[:8] == "https://" {
		return s[8:]
	}
	return s
}
