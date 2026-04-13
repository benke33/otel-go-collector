package otel

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"github.com/benke33/gitlab-otel-exporter/internal/config"
)

// InitLogger initializes OpenTelemetry log provider
func InitLogger(ctx context.Context, cfg *config.Config) (*sdklog.LoggerProvider, error) {
	endpoint := cfg.GetEndpoint()

	exporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpoint(endpoint),
		otlploghttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create log exporter: %w", err)
	}

	serviceName := fmt.Sprintf("%s/%s",
		os.Getenv("CI_PROJECT_NAMESPACE"),
		os.Getenv("CI_PROJECT_NAME"))

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(os.Getenv("CI_COMMIT_SHA")),
		),
	)
	if err != nil {
		return nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	return lp, nil
}

// NewLogger creates a named logger from the global provider
func NewLogger(name string) otellog.Logger {
	return global.GetLoggerProvider().Logger(name)
}
