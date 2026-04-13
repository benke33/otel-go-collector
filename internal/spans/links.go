package spans

import (
	"os"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// createSpanLinks creates links to related spans for SigNoz linked spans view
func (e *Exporter) createSpanLinks() []trace.Link {
	var links []trace.Link

	// Link to parent pipeline if this is a downstream pipeline
	if parentTraceID := os.Getenv("TRACEPARENT"); parentTraceID != "" {
		if traceID, spanID := parseTraceParent(parentTraceID); traceID.IsValid() && spanID.IsValid() {
			links = append(links, trace.Link{
				SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
					TraceID: traceID,
					SpanID:  spanID,
				}),
				Attributes: []attribute.KeyValue{
					attribute.String("link.type", "parent_pipeline"),
					attribute.String("parent.pipeline.url", os.Getenv("PARENT_PIPELINE_URL")),
				},
			})
		}
	}

	return links
}

// parseTraceParent extracts trace ID and span ID from W3C traceparent header
func parseTraceParent(traceparent string) (trace.TraceID, trace.SpanID) {
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		return trace.TraceID{}, trace.SpanID{}
	}

	traceID, err := trace.TraceIDFromHex(parts[1])
	if err != nil {
		return trace.TraceID{}, trace.SpanID{}
	}

	spanID, err := trace.SpanIDFromHex(parts[2])
	if err != nil {
		return trace.TraceID{}, trace.SpanID{}
	}

	return traceID, spanID
}
