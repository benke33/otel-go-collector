package spans

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
	"github.com/benke33/gitlab-otel-exporter/internal/config"
	"github.com/benke33/gitlab-otel-exporter/internal/gitlab"
	otelutil "github.com/benke33/gitlab-otel-exporter/internal/otel"
	"github.com/benke33/gitlab-otel-exporter/internal/utils"
	"github.com/benke33/gitlab-otel-exporter/pkg/semconv"
)

// Exporter handles span creation and export
type Exporter struct {
	config          *config.Config
	gitClient       *gitlab.Client
	tracer          trace.Tracer
	logger          otellog.Logger
	pipelineEndTime *time.Time
}

// NewExporter creates a new span exporter
func NewExporter(cfg *config.Config, gitClient *gitlab.Client) *Exporter {
	return &Exporter{
		config:    cfg,
		gitClient: gitClient,
		tracer:    otel.Tracer("gitlab-ci-collector"),
		logger:    otelutil.NewLogger("gitlab-ci-collector"),
	}
}

// ExportPipeline exports traces for the entire pipeline
func (e *Exporter) ExportPipeline(ctx context.Context) error {
	fmt.Println("Fetching pipeline data...")
	pipeline, err := e.gitClient.FetchPipeline()
	if err != nil {
		return err
	}

	// Check for parent pipeline context
	ctx = otelutil.ExtractParentContext(ctx, e.gitClient, pipeline)

	jobs, err := e.gitClient.FetchJobs()
	if err != nil {
		return err
	}
	fmt.Printf("Found %d jobs in pipeline\n", len(jobs))

	// Create pipeline span with job timing context
	ctx, pipelineSpan := e.createPipelineSpan(ctx, pipeline, jobs)
	defer e.endPipelineSpan(pipelineSpan, pipeline)

	// Export trace context for downstream pipelines
	otelutil.ExportTraceContext(ctx, e.config.Debug)

	// Create job spans
	fmt.Println("Creating job spans...")
	for _, job := range jobs {
		if job.Status == "skipped" {
			continue
		}
		if err := e.createJobSpan(ctx, job); err != nil {
			log.Printf("failed to export job span for job %d: %v", job.ID, err)
		}
	}

	// Emit correlated pipeline log
	e.emitLog(ctx, "Pipeline export completed", otellog.SeverityInfo,
		otellog.String("cicd.pipeline.run.id", os.Getenv("CI_PIPELINE_ID")),
		otellog.Int("gitlab.job.count", len(jobs)),
	)

	return nil
}

func (e *Exporter) createPipelineSpan(ctx context.Context, pipeline *gitlab.PipelineData, jobs []*gitlab.JobData) (context.Context, trace.Span) {
	pipelineName := fmt.Sprintf("%s/%s #%d",
		os.Getenv("CI_PROJECT_NAMESPACE"),
		os.Getenv("CI_PROJECT_NAME"),
		pipeline.ID)

	pipelineAttrs := semconv.PipelineAttributes()
	pipelineAttrs = append(pipelineAttrs, utils.FlattenMap("", pipeline.Raw)...)

	// Add parent pipeline correlation attributes
	if parentAttrs := semconv.ParentPipelineAttributes(e.gitClient, pipeline); len(parentAttrs) > 0 {
		pipelineAttrs = append(pipelineAttrs, parentAttrs...)
	}

	// Calculate pipeline span timing from jobs to ensure proper hierarchy
	var pipelineStart, pipelineEnd *time.Time
	for _, job := range jobs {
		if job.StartedAt != nil && (pipelineStart == nil || job.StartedAt.Before(*pipelineStart)) {
			pipelineStart = job.StartedAt
		}
		if job.FinishedAt != nil && (pipelineEnd == nil || job.FinishedAt.After(*pipelineEnd)) {
			pipelineEnd = job.FinishedAt
		}
	}

	// Fallback to pipeline timestamps if no job timestamps
	if pipelineStart == nil && pipeline.CreatedAt != nil {
		pipelineStart = pipeline.CreatedAt
	}
	if pipelineEnd == nil && pipeline.UpdatedAt != nil {
		pipelineEnd = pipeline.UpdatedAt
	}

	var startOpts []trace.SpanStartOption
	if pipelineStart != nil {
		startOpts = append(startOpts, trace.WithTimestamp(*pipelineStart))
	}
	startOpts = append(startOpts,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(pipelineAttrs...),
	)

	if links := e.createSpanLinks(); len(links) > 0 {
		startOpts = append(startOpts, trace.WithLinks(links...))
	}

	ctx, pipelineSpan := e.tracer.Start(ctx, pipelineName, startOpts...)
	spanCtx := pipelineSpan.SpanContext()
	fmt.Printf("Creating pipeline span: %s (trace_id: %s, span_id: %s)\n",
		pipelineName, spanCtx.TraceID().String(), spanCtx.SpanID().String())
	if pipelineStart != nil && pipelineEnd != nil {
		fmt.Printf("Pipeline timing: %s -> %s\n",
			pipelineStart.Format("15:04:05"), pipelineEnd.Format("15:04:05"))
	}

	e.pipelineEndTime = pipelineEnd

	return ctx, pipelineSpan
}

func (e *Exporter) endPipelineSpan(pipelineSpan trace.Span, pipeline *gitlab.PipelineData) {
	pipelineSpan.SetAttributes(semconv.PipelineResult(pipeline.Status))
	if pipeline.Status == "failed" {
		pipelineSpan.SetStatus(codes.Error, "pipeline failed")
	} else {
		pipelineSpan.SetStatus(codes.Ok, "")
	}

	// Use calculated end time or fallback to pipeline timestamp
	if e.pipelineEndTime != nil {
		pipelineSpan.End(trace.WithTimestamp(*e.pipelineEndTime))
	} else if pipeline.UpdatedAt != nil {
		pipelineSpan.End(trace.WithTimestamp(*pipeline.UpdatedAt))
	} else {
		pipelineSpan.End()
	}
}

func (e *Exporter) createJobSpan(ctx context.Context, job *gitlab.JobData) error {
	if job.StartedAt == nil || job.FinishedAt == nil {
		return nil
	}

	spanName := job.Name // Simplified name
	attrs := semconv.JobAttributes(job)

	// Create job span as child of pipeline span with proper timing
	jobCtx, jobSpan := e.tracer.Start(ctx, spanName,
		trace.WithTimestamp(*job.StartedAt),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	jobSpanCtx := jobSpan.SpanContext()
	fmt.Printf("  Job: %s (%s) [%s -> %s] (parent: %s, span: %s)\n",
		job.Name, job.Status,
		job.StartedAt.Format("15:04:05"), job.FinishedAt.Format("15:04:05"),
		jobSpanCtx.TraceID().String(), jobSpanCtx.SpanID().String())

	if job.Status == "failed" {
		jobSpan.SetStatus(codes.Error, "job failed")
	} else {
		jobSpan.SetStatus(codes.Ok, "job completed")
	}

	// End span with correct timestamp
	jobSpan.End(trace.WithTimestamp(*job.FinishedAt))

	// Emit correlated job log with full trace output
	severity := otellog.SeverityInfo
	if job.Status == "failed" {
		severity = otellog.SeverityError
	}
	var jobTrace string
	if e.gitClient != nil {
		var err error
		jobTrace, err = e.gitClient.FetchJobTrace(int(job.ID))
		if err != nil {
			fmt.Printf("  Warning: failed to fetch trace for job %d: %v\n", job.ID, err)
		}
	}
	if jobTrace == "" {
		jobTrace = fmt.Sprintf("Job %s %s", job.Name, job.Status)
	}
	e.emitLog(jobCtx, jobTrace, severity,
		otellog.String("cicd.pipeline.task.name", job.Name),
		otellog.String("cicd.pipeline.task.run.id", fmt.Sprintf("%d", job.ID)),
		otellog.String("gitlab.job.stage", job.Stage),
	)

	return nil
}

// emitLog emits a log record correlated with the current span context
func (e *Exporter) emitLog(ctx context.Context, msg string, severity otellog.Severity, attrs ...otellog.KeyValue) {
	if e.logger == nil {
		return
	}
	var record otellog.Record
	record.SetBody(otellog.StringValue(msg))
	record.SetSeverity(severity)
	record.AddAttributes(attrs...)
	e.logger.Emit(ctx, record)
}

// CreateRootSpan creates a root span for the pipeline without jobs
func (e *Exporter) CreateRootSpan(ctx context.Context, pipeline *gitlab.PipelineData) (context.Context, trace.Span, error) {
	// Check for parent pipeline context
	ctx = otelutil.ExtractParentContext(ctx, e.gitClient, pipeline)

	pipelineName := fmt.Sprintf("%s/%s #%d",
		os.Getenv("CI_PROJECT_NAMESPACE"),
		os.Getenv("CI_PROJECT_NAME"),
		pipeline.ID)

	pipelineAttrs := semconv.PipelineAttributes()
	pipelineAttrs = append(pipelineAttrs, utils.FlattenMap("", pipeline.Raw)...)

	// Add parent pipeline correlation attributes
	if parentAttrs := semconv.ParentPipelineAttributes(e.gitClient, pipeline); len(parentAttrs) > 0 {
		pipelineAttrs = append(pipelineAttrs, parentAttrs...)
	}

	var startOpts []trace.SpanStartOption
	if pipeline.CreatedAt != nil {
		startOpts = append(startOpts, trace.WithTimestamp(*pipeline.CreatedAt))
	}
	startOpts = append(startOpts,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(pipelineAttrs...),
	)

	if links := e.createSpanLinks(); len(links) > 0 {
		startOpts = append(startOpts, trace.WithLinks(links...))
	}

	ctx, rootSpan := e.tracer.Start(ctx, pipelineName, startOpts...)
	spanCtx := rootSpan.SpanContext()
	fmt.Printf("Creating root span: %s (trace_id: %s, span_id: %s)\n",
		pipelineName, spanCtx.TraceID().String(), spanCtx.SpanID().String())

	return ctx, rootSpan, nil
}
