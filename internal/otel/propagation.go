package otel

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"github.com/benke33/gitlab-otel-exporter/internal/gitlab"
)

// ExtractParentContext extracts trace context from parent pipeline
func ExtractParentContext(ctx context.Context, gitClient *gitlab.Client, pipeline *gitlab.PipelineData) context.Context {
	// Look for parent pipeline trace context in variables (check TRACEPARENT first)
	if traceParent := os.Getenv("TRACEPARENT"); traceParent != "" {
		carrier := propagation.MapCarrier{"traceparent": traceParent}
		return otel.GetTextMapPropagator().Extract(ctx, carrier)
	}

	// Check if this pipeline was triggered by another pipeline
	if os.Getenv("CI_PIPELINE_SOURCE") != "pipeline" && os.Getenv("CI_PIPELINE_SOURCE") != "trigger" {
		return ctx
	}

	// Try to extract from pipeline variables if available
	projectID := os.Getenv("CI_PROJECT_ID")
	pipelineID, _ := strconv.ParseInt(os.Getenv("CI_PIPELINE_ID"), 10, 64)

	if variables, _, err := gitClient.GetClient().Pipelines.GetPipelineVariables(projectID, pipelineID, nil); err == nil {
		for _, v := range variables {
			if v.Key == "TRACEPARENT" {
				carrier := propagation.MapCarrier{"traceparent": v.Value}
				return otel.GetTextMapPropagator().Extract(ctx, carrier)
			}
		}
	}

	return ctx
}

// ExportTraceContext exports current trace context for downstream pipelines
func ExportTraceContext(ctx context.Context, debug bool) {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if traceParent := carrier["traceparent"]; traceParent != "" {
		fmt.Printf("TRACE_PARENT=%s\n", traceParent)
		if debug {
			fmt.Printf("   Use this in downstream pipeline variables\n")
		}
	}
}
