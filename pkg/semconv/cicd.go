package semconv

import (
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"github.com/benke33/gitlab-otel-exporter/internal/gitlab"
	"github.com/benke33/gitlab-otel-exporter/internal/utils"
)

// PipelineAttributes returns CI/CD semantic convention attributes for pipeline
func PipelineAttributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.CICDPipelineName(getPipelineName()),
		semconv.CICDPipelineRunID(os.Getenv("CI_PIPELINE_ID")),
		semconv.CICDPipelineRunURLFull(os.Getenv("CI_PIPELINE_URL")),
		semconv.VCSRepositoryURLFull(os.Getenv("CI_PROJECT_URL")),
		semconv.VCSRefHeadName(os.Getenv("CI_COMMIT_REF_NAME")),
		semconv.VCSRefHeadRevision(os.Getenv("CI_COMMIT_SHA")),
		refHeadType(),
		attribute.String("gitlab.pipeline.trigger.type", triggerType()),
	}

	if user := os.Getenv("GITLAB_USER_LOGIN"); user != "" {
		attrs = append(attrs, attribute.String("gitlab.pipeline.trigger.user", user))
	}

	return attrs
}

// PipelineResult returns the cicd.pipeline.result attribute for a pipeline status
func PipelineResult(status string) attribute.KeyValue {
	switch status {
	case "success":
		return semconv.CICDPipelineResultSuccess
	case "failed":
		return semconv.CICDPipelineResultFailure
	case "canceled":
		return semconv.CICDPipelineResultCancellation
	case "skipped":
		return semconv.CICDPipelineResultSkip
	default:
		return semconv.CICDPipelineResultError
	}
}

// JobAttributes returns CI/CD semantic convention attributes for job
func JobAttributes(job *gitlab.JobData) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.CICDPipelineTaskName(job.Name),
		semconv.CICDPipelineTaskRunID(fmt.Sprintf("%d", job.ID)),
		semconv.CICDPipelineTaskRunURLFull(job.WebURL),
		taskType(job.Stage),
		TaskRunResult(job.Status),
		attribute.String("gitlab.job.stage", job.Stage),
	}

	attrs = append(attrs, utils.FlattenMap("", job.Raw)...)
	return attrs
}

// TaskRunResult returns the cicd.pipeline.task.run.result attribute for a job status
func TaskRunResult(status string) attribute.KeyValue {
	switch status {
	case "success":
		return semconv.CICDPipelineTaskRunResultSuccess
	case "failed":
		return semconv.CICDPipelineTaskRunResultFailure
	case "canceled":
		return semconv.CICDPipelineTaskRunResultCancellation
	case "skipped":
		return semconv.CICDPipelineTaskRunResultSkip
	default:
		return semconv.CICDPipelineTaskRunResultError
	}
}

// ParentPipelineAttributes returns attributes for parent pipeline correlation
func ParentPipelineAttributes(gitClient *gitlab.Client, pipeline *gitlab.PipelineData) []attribute.KeyValue {
	var attrs []attribute.KeyValue

	if os.Getenv("CI_PIPELINE_SOURCE") == "pipeline" || os.Getenv("CI_PIPELINE_SOURCE") == "trigger" {
		if id := os.Getenv("CI_PARENT_PIPELINE_ID"); id != "" {
			attrs = append(attrs, attribute.String("gitlab.pipeline.parent.id", id))
		}
		if id := os.Getenv("CI_PARENT_PROJECT_ID"); id != "" {
			attrs = append(attrs, attribute.String("gitlab.pipeline.parent.project.id", id))
		}
		if pipeline.User != nil && pipeline.User.ID != 0 {
			attrs = append(attrs, attribute.String("gitlab.pipeline.trigger.user.id", fmt.Sprintf("%d", pipeline.User.ID)))
		}
	}

	return attrs
}

func refHeadType() attribute.KeyValue {
	if os.Getenv("CI_COMMIT_TAG") != "" {
		return semconv.VCSRefHeadTypeTag
	}
	return semconv.VCSRefHeadTypeBranch
}

func taskType(stage string) attribute.KeyValue {
	switch stage {
	case "build":
		return semconv.CICDPipelineTaskTypeBuild
	case "test", "lint", "security", "integration":
		return semconv.CICDPipelineTaskTypeTest
	case "deploy":
		return semconv.CICDPipelineTaskTypeDeploy
	default:
		return semconv.CICDPipelineTaskTypeKey.String(stage)
	}
}

func triggerType() string {
	switch os.Getenv("CI_PIPELINE_SOURCE") {
	case "push":
		return "scm.push"
	case "merge_request_event":
		return "scm.pull_request"
	case "schedule":
		return "schedule"
	case "trigger", "pipeline":
		return "other_pipeline"
	case "web":
		return "manual"
	default:
		return "manual"
	}
}

func getPipelineName() string {
	if name := os.Getenv("CI_PIPELINE_NAME"); name != "" {
		return name
	}
	return fmt.Sprintf("%s/%s", os.Getenv("CI_PROJECT_NAMESPACE"), os.Getenv("CI_PROJECT_NAME"))
}
