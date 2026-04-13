package spans

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"go.opentelemetry.io/otel"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"github.com/benke33/gitlab-otel-exporter/internal/config"
	gitlabpkg "github.com/benke33/gitlab-otel-exporter/internal/gitlab"
	"github.com/benke33/gitlab-otel-exporter/pkg/semconv"
)

// inMemoryLogProcessor captures log records for testing
type inMemoryLogProcessor struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (p *inMemoryLogProcessor) OnEmit(ctx context.Context, record *sdklog.Record) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.records = append(p.records, *record)
	return nil
}

func (p *inMemoryLogProcessor) Shutdown(ctx context.Context) error { return nil }
func (p *inMemoryLogProcessor) ForceFlush(ctx context.Context) error { return nil }
func (p *inMemoryLogProcessor) Enabled(ctx context.Context, param sdklog.EnabledParameters) bool {
	return true
}

func (p *inMemoryLogProcessor) Records() []sdklog.Record {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]sdklog.Record, len(p.records))
	copy(out, p.records)
	return out
}

func TestCreateJobSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	cfg := &config.Config{Debug: false}
	spanExporter := &Exporter{
		config: cfg,
		tracer: otel.Tracer("test"),
	}

	ctx := context.Background()
	now := time.Now()
	started := now.Add(-5 * time.Minute)
	job := &gitlabpkg.JobData{
		Job: &gitlab.Job{
			ID:         123,
			Name:       "build",
			Stage:      "build",
			Status:     "success",
			WebURL:     "https://gitlab.com/test/job/123",
			StartedAt:  &started,
			FinishedAt: &now,
		},
		Raw: map[string]interface{}{
			"id":    float64(123),
			"name":  "build",
			"stage": "build",
		},
	}

	err := spanExporter.createJobSpan(ctx, job)
	if err != nil {
		t.Errorf("createJobSpan should not error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Errorf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "build" {
		t.Errorf("unexpected span name: %s", spans[0].Name)
	}
}

func TestCreateJobSpanWithNilTimestamps(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	cfg := &config.Config{Debug: false}
	spanExporter := &Exporter{
		config: cfg,
		tracer: otel.Tracer("test"),
	}

	ctx := context.Background()
	job := &gitlabpkg.JobData{
		Job: &gitlab.Job{
			ID:         123,
			Name:       "build",
			Stage:      "build",
			Status:     "pending",
			StartedAt:  nil,
			FinishedAt: nil,
		},
		Raw: map[string]interface{}{},
	}

	err := spanExporter.createJobSpan(ctx, job)
	if err != nil {
		t.Errorf("createJobSpan with nil timestamps should not error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 0 {
		t.Errorf("expected 0 spans for job with nil timestamps, got %d", len(spans))
	}
}

func TestDownstreamPipelineIntegration(t *testing.T) {
	// Simulate downstream pipeline environment
	_ = os.Setenv("CI_PIPELINE_SOURCE", "pipeline")
	_ = os.Setenv("CI_PARENT_PIPELINE_ID", "100")
	_ = os.Setenv("CI_PARENT_PROJECT_ID", "200")
	_ = os.Setenv("TRACEPARENT", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	_ = os.Setenv("CI_PROJECT_NAMESPACE", "test")
	_ = os.Setenv("CI_PROJECT_NAME", "downstream")
	defer func() {
		_ = os.Unsetenv("CI_PIPELINE_SOURCE")
		_ = os.Unsetenv("CI_PARENT_PIPELINE_ID")
		_ = os.Unsetenv("CI_PARENT_PROJECT_ID")
		_ = os.Unsetenv("TRACEPARENT")
		_ = os.Unsetenv("CI_PROJECT_NAMESPACE")
		_ = os.Unsetenv("CI_PROJECT_NAME")
	}()

	// Test pipeline attributes include parent info
	attrs := semconv.PipelineAttributes()
	found := false
	for _, attr := range attrs {
		if attr.Key == "gitlab.pipeline.trigger.type" && attr.Value.AsString() == "other_pipeline" {
			found = true
			break
		}
	}
	if !found {
		t.Error("downstream pipeline should have trigger.type = other_pipeline")
	}

	// Test parent attributes are generated
	pipeline := &gitlabpkg.PipelineData{
		Pipeline: &gitlab.Pipeline{
			User: &gitlab.BasicUser{ID: 300},
		},
	}
	parentAttrs := semconv.ParentPipelineAttributes(nil, pipeline)
	if len(parentAttrs) == 0 {
		t.Error("downstream pipeline should have parent attributes")
	}
}

func TestLogTraceCorrelation(t *testing.T) {
	// Set up in-memory trace exporter
	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(spanExporter),
	)
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	// Set up in-memory log processor
	logProcessor := &inMemoryLogProcessor{}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(logProcessor),
	)
	global.SetLoggerProvider(lp)
	defer func() { _ = lp.Shutdown(context.Background()) }()

	cfg := &config.Config{Debug: false}
	e := &Exporter{
		config: cfg,
		tracer: otel.Tracer("test"),
		logger: global.GetLoggerProvider().Logger("test"),
	}

	// Create a span and emit a log within its context
	ctx, span := e.tracer.Start(context.Background(), "test-span")
	expectedTraceID := span.SpanContext().TraceID()
	expectedSpanID := span.SpanContext().SpanID()

	e.emitLog(ctx, "test log message", otellog.SeverityInfo,
		otellog.String("test.key", "test.value"),
	)
	span.End()

	// Verify log was recorded with trace correlation
	records := logProcessor.Records()
	if len(records) == 0 {
		t.Fatal("expected at least one log record")
	}

	record := records[0]
	if record.Body().AsString() != "test log message" {
		t.Errorf("log body = %q, want %q", record.Body().AsString(), "test log message")
	}
	if record.TraceID() != expectedTraceID {
		t.Errorf("log trace_id = %s, want %s", record.TraceID(), expectedTraceID)
	}
	if record.SpanID() != expectedSpanID {
		t.Errorf("log span_id = %s, want %s", record.SpanID(), expectedSpanID)
	}
	if record.Severity() != otellog.SeverityInfo {
		t.Errorf("log severity = %v, want %v", record.Severity(), otellog.SeverityInfo)
	}
}

func TestLogWithoutSpanContext(t *testing.T) {
	// Set up in-memory log processor
	logProcessor := &inMemoryLogProcessor{}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(logProcessor),
	)
	global.SetLoggerProvider(lp)
	defer func() { _ = lp.Shutdown(context.Background()) }()

	e := &Exporter{
		config: &config.Config{},
		logger: global.GetLoggerProvider().Logger("test"),
	}

	// Emit log without any span context
	e.emitLog(context.Background(), "orphan log", otellog.SeverityWarn)

	records := logProcessor.Records()
	if len(records) == 0 {
		t.Fatal("expected at least one log record")
	}

	record := records[0]
	if record.Body().AsString() != "orphan log" {
		t.Errorf("log body = %q, want %q", record.Body().AsString(), "orphan log")
	}
	if record.TraceID().IsValid() {
		t.Errorf("orphan log should have zero trace_id, got %s", record.TraceID())
	}
	if record.SpanID().IsValid() {
		t.Errorf("orphan log should have zero span_id, got %s", record.SpanID())
	}
}
