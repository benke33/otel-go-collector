package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/benke33/gitlab-otel-exporter/internal/config"
	"github.com/benke33/gitlab-otel-exporter/internal/gitlab"
	"github.com/benke33/gitlab-otel-exporter/internal/otel"
	"github.com/benke33/gitlab-otel-exporter/internal/spans"
)

func main() {
	var createRootSpan bool
	var useRootSpan string

	flag.BoolVar(&createRootSpan, "create-root-span", false, "Create root span only")
	flag.StringVar(&useRootSpan, "use-root-span", "", "Use existing root span (TRACEPARENT format)")
	flag.Parse()

	ctx := context.Background()

	fmt.Println("Starting GitLab OpenTelemetry Exporter")

	// Load configuration
	cfg := config.Load()

	// Initialize tracer
	tp, err := otel.InitTracer(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("error shutting down tracer: %v", err)
		}
	}()

	// Initialize logger
	lp, err := otel.InitLogger(ctx, cfg)
	if err != nil {
		log.Printf("warning: failed to initialize logger: %v", err)
	} else {
		defer func() {
			if err := lp.Shutdown(ctx); err != nil {
				log.Printf("error shutting down logger: %v", err)
			}
		}()
	}

	// Create GitLab client
	gitClient, err := gitlab.NewClient(cfg)
	if err != nil {
		log.Fatalf("failed to create GitLab client: %v", err)
	}

	// Handle different modes
	if createRootSpan {
		if err := createRootSpanOnly(ctx, cfg, gitClient); err != nil {
			log.Fatalf("failed to create root span: %v", err)
		}
		return
	}

	if useRootSpan != "" {
		// Set TRACEPARENT environment variable for context extraction
		fmt.Printf("Setting TRACEPARENT: %s\n", useRootSpan)
		_ = os.Setenv("TRACEPARENT", useRootSpan)
		fmt.Printf("TRACEPARENT env var: %s\n", os.Getenv("TRACEPARENT"))
	}

	// Create and run exporter
	exporter := spans.NewExporter(cfg, gitClient)
	if err := exporter.ExportPipeline(ctx); err != nil {
		log.Fatalf("failed to export trace: %v", err)
	}

	fmt.Println("Traces exported successfully")
}

func createRootSpanOnly(ctx context.Context, cfg *config.Config, gitClient *gitlab.Client) error {
	// Create a simple root span for the pipeline
	pipeline, err := gitClient.FetchPipeline()
	if err != nil {
		return err
	}

	exporter := spans.NewExporter(cfg, gitClient)
	rootCtx, rootSpan, err := exporter.CreateRootSpan(ctx, pipeline)
	if err != nil {
		return err
	}
	defer rootSpan.End()

	// Export the trace context
	if spanCtx := rootSpan.SpanContext(); spanCtx.IsValid() {
		traceparent := fmt.Sprintf("00-%s-%s-01",
			spanCtx.TraceID().String(),
			spanCtx.SpanID().String())
		fmt.Printf("TRACE_PARENT=%s\n", traceparent)
	}

	_ = rootCtx
	return nil
}
