# GitLab OpenTelemetry Exporter

A minimal OpenTelemetry exporter for GitLab CI/CD pipelines that exports traces and correlated logs following the [CI/CD semantic conventions](https://opentelemetry.io/docs/specs/semconv/cicd/).

## Features

- Exports traces and logs to any OTLP-compatible backend (SigNoz, Jaeger, etc.)
- **Trace-log correlation** — logs are emitted within span context for automatic correlation in observability UIs
- **Full job log capture** — fetches complete job output from GitLab API and sends as correlated log records
- **Root span support** — creates root span in `.pre` stage for proper trace hierarchy across all pipeline stages
- **Downstream pipeline correlation** — automatically links triggered pipelines to parent traces via TRACEPARENT propagation
- Follows [OpenTelemetry CI/CD semantic conventions v1.40.0](https://opentelemetry.io/docs/specs/semconv/cicd/)
- Parent-child span relationships between pipeline and job spans
- Span links for cross-trace references
- Pipeline and task result attributes (`cicd.pipeline.result`, `cicd.pipeline.task.run.result`)
- Comprehensive metadata export (all GitLab API data flattened as span attributes)
- ANSI escape code stripping for clean attribute values
- Supports HTTP, gRPC, and stdout OTLP protocols
- Written in Go 1.26

## Quick Start

```bash
make build
make test
```

## Usage

### Pre-built Docker Image

The recommended way to run the exporter in CI is using the pre-built Docker image:

```yaml
variables:
  OTEL_EXPORTER_OTLP_ENDPOINT: "otel-collector:4318"

otel-init:
  stage: .pre
  image: armdocker.rnd.ericsson.se/proj-bosgitops/gitlab-otel-exporter:latest
  script:
    - export GITLAB_TOKEN=${GITLAB_SECRET_TOKEN}
    - export GITLAB_SERVER_URL=https://gitlab.example.com
    - gitlab-otel-exporter --create-root-span | tee root_trace.log
    - grep TRACE_PARENT root_trace.log > root_trace.env || echo "TRACE_PARENT=" > root_trace.env
  artifacts:
    reports:
      dotenv: root_trace.env
  when: always
  allow_failure: true

otel-export:
  stage: .post
  image: armdocker.rnd.ericsson.se/proj-bosgitops/gitlab-otel-exporter:latest
  dependencies:
    - otel-init
  script:
    - export GITLAB_TOKEN=${GITLAB_SECRET_TOKEN}
    - export GITLAB_SERVER_URL=https://gitlab.example.com
    - gitlab-otel-exporter --use-root-span=$TRACE_PARENT
  when: always
  allow_failure: true
```

### Environment Variables

| Variable | Description | Required |
|---|---|---|
| `GITLAB_TOKEN` | GitLab API token with `read_api` scope | Yes |
| `GITLAB_SERVER_URL` | GitLab server URL (falls back to `CI_SERVER_URL`) | No |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint (host:port) | Yes |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | Protocol: `http` (default), `grpc`, or `stdout` | No |
| `DEBUG` | Set to `true` for debug output | No |

### Root Span Mode

The exporter supports a two-phase approach for proper trace hierarchy:

1. **`.pre` stage** — `--create-root-span` creates a root span before any jobs run and outputs a `TRACE_PARENT` value
2. **`.post` stage** — `--use-root-span=<traceparent>` uses the root span as parent, ensuring all job spans share the same trace ID

The `.post` job uses `dependencies` (not `needs`) to receive the dotenv artifact from the `.pre` job. This ensures proper stage ordering without bypassing the pipeline DAG.

### Downstream Pipeline Correlation

Trace context is propagated to downstream pipelines using GitLab's `trigger` keyword:

```yaml
trigger-downstream:
  stage: deploy
  trigger:
    project: group/downstream-project
    branch: main
    strategy: depend
  variables:
    TRACEPARENT: $TRACE_PARENT
```

The exporter automatically detects and correlates downstream pipelines when:
- `TRACEPARENT` environment variable is present
- `CI_PIPELINE_SOURCE` is "pipeline" or "trigger"

### Protocol Configuration

```yaml
# HTTP (default) - port 4318
OTEL_EXPORTER_OTLP_PROTOCOL: "http"
OTEL_EXPORTER_OTLP_ENDPOINT: "collector:4318"

# gRPC - port 4317
OTEL_EXPORTER_OTLP_PROTOCOL: "grpc"
OTEL_EXPORTER_OTLP_ENDPOINT: "collector:4317"

# Console/stdout - for debugging
OTEL_EXPORTER_OTLP_PROTOCOL: "stdout"
```

The endpoint can be specified with or without the `http://` scheme — it will be stripped automatically.

## Trace-Log Correlation

The exporter sends logs via the OpenTelemetry Log SDK with automatic trace correlation. Each job span emits a correlated log record containing:

- **Body**: Full job trace output (complete CI job console log)
- **Attributes**: `cicd.pipeline.task.name`, `cicd.pipeline.task.run.id`, `gitlab.job.stage`
- **Correlation**: Automatic via span context — `trace_id` and `span_id` are set by the OTel SDK

In SigNoz, this enables clicking from a trace to see the full job logs, and from logs back to the originating trace.

## Exported Attributes

All attributes follow the [OpenTelemetry CI/CD semantic conventions v1.40.0](https://opentelemetry.io/docs/specs/semconv/cicd/) where available. GitLab-specific attributes that are not part of the spec use the `gitlab.*` namespace.

### Pipeline Span

| Attribute | Source | Description |
|---|---|---|
| `cicd.pipeline.name` | Spec | Pipeline name or project path |
| `cicd.pipeline.run.id` | Spec | Pipeline ID |
| `cicd.pipeline.run.url.full` | Spec | Pipeline URL |
| `cicd.pipeline.result` | Spec | Pipeline result (`success`, `failure`, `error`, `cancellation`, `skip`) |
| `vcs.repository.url.full` | Spec | Repository URL |
| `vcs.ref.head.name` | Spec | Branch or tag name |
| `vcs.ref.head.revision` | Spec | Commit SHA |
| `vcs.ref.head.type` | Spec | Reference type (`branch` or `tag`) |
| `gitlab.pipeline.trigger.type` | GitLab | Trigger type (scm.push, scm.pull_request, schedule, other_pipeline, manual) |
| `gitlab.pipeline.trigger.user` | GitLab | User who triggered the pipeline |
| `gitlab.pipeline.parent.id` | GitLab | Parent pipeline ID (downstream only) |
| `gitlab.pipeline.parent.project.id` | GitLab | Parent project ID (downstream only) |

### Job Span

| Attribute | Source | Description |
|---|---|---|
| `cicd.pipeline.task.name` | Spec | Job name |
| `cicd.pipeline.task.run.id` | Spec | Job ID |
| `cicd.pipeline.task.run.url.full` | Spec | Job URL |
| `cicd.pipeline.task.type` | Spec | Task type (`build`, `test`, `deploy`) |
| `cicd.pipeline.task.run.result` | Spec | Task result (`success`, `failure`, `error`, `cancellation`, `skip`) |
| `gitlab.job.stage` | GitLab | GitLab stage name |

All GitLab API metadata is also flattened and included as span attributes.

## Development

### Make Targets

```
make build           Build the binary
make test            Run tests with coverage
make test-report     Run tests and generate JUnit XML report
make lint            Run all linters (fmt + vet)
make docker          Build Docker image
make clean           Remove build artifacts
make tidy            Tidy and verify dependencies
make help            Show all targets
```

### Docker

```bash
docker build -t gitlab-otel-exporter .
docker run -e OTEL_EXPORTER_OTLP_ENDPOINT=collector:4318 gitlab-otel-exporter
```

## License

Apache 2.0
