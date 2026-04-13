package gitlab

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"github.com/benke33/gitlab-otel-exporter/internal/config"
	"github.com/benke33/gitlab-otel-exporter/internal/utils"
)

// Client wraps GitLab API client with configuration
type Client struct {
	client *gitlab.Client
	config *config.Config
}

// NewClient creates a new GitLab client
func NewClient(cfg *config.Config) (*Client, error) {
	client, err := gitlab.NewClient(cfg.Token, gitlab.WithBaseURL(cfg.ServerURL))
	if err != nil {
		return nil, err
	}

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// FetchPipeline retrieves pipeline data from GitLab API
func (c *Client) FetchPipeline() (*PipelineData, error) {
	pipelineID, _ := strconv.ParseInt(c.config.PipelineID, 10, 64)

	pipeline, resp, err := c.client.Pipelines.GetPipeline(c.config.ProjectID, pipelineID, nil)
	if err != nil {
		return nil, checkAPIError("GetPipeline", resp, err)
	}

	raw, err := utils.StructToMap(pipeline)
	if err != nil {
		return nil, err
	}
	utils.CleanRaw(raw)

	return &PipelineData{Pipeline: pipeline, Raw: raw}, nil
}

// FetchJobs retrieves all jobs for the pipeline
func (c *Client) FetchJobs() ([]*JobData, error) {
	pipelineID, _ := strconv.ParseInt(c.config.PipelineID, 10, 64)

	jobs, resp, err := c.client.Jobs.ListPipelineJobs(c.config.ProjectID, pipelineID, &gitlab.ListJobsOptions{}, nil)
	if err != nil {
		return nil, checkAPIError("ListPipelineJobs", resp, err)
	}

	var jobData []*JobData
	for _, job := range jobs {
		raw, err := utils.StructToMap(job)
		if err != nil {
			log.Printf("failed to convert job %d to map: %v", job.ID, err)
			continue
		}
		utils.CleanRaw(raw)
		jobData = append(jobData, &JobData{Job: job, Raw: raw})
	}

	return jobData, nil
}

// GetClient returns the underlying GitLab client
func (c *Client) GetClient() *gitlab.Client {
	return c.client
}

// FetchJobTrace retrieves the log output for a job
func (c *Client) FetchJobTrace(jobID int) (string, error) {
	reader, resp, err := c.client.Jobs.GetTraceFile(c.config.ProjectID, int64(jobID), nil)
	if err != nil {
		return "", checkAPIError(fmt.Sprintf("GetTraceFile(job=%d)", jobID), resp, err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return "", err
	}
	return utils.StripANSI(buf.String()), nil
}

// checkAPIError wraps API errors with HTTP status for better diagnostics
func checkAPIError(operation string, resp *gitlab.Response, err error) error {
	if resp != nil && resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s: HTTP %d %s - %w", operation, resp.StatusCode, http.StatusText(resp.StatusCode), err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
