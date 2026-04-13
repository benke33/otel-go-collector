package config

import (
	"os"
	"strings"
)

// Config holds all configuration for the exporter
type Config struct {
	// OTLP Configuration
	Protocol string
	Endpoint string

	// GitLab Configuration
	Token      string
	ServerURL  string
	ProjectID  string
	PipelineID string

	// Debug settings
	Debug bool
}

// Load creates a new configuration from environment variables
func Load() *Config {
	return &Config{
		Protocol:   getEnv("OTEL_EXPORTER_OTLP_PROTOCOL", "http"),
		Endpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		Token:      os.Getenv("GITLAB_TOKEN"),
		ServerURL:  getEnv("GITLAB_SERVER_URL", os.Getenv("CI_SERVER_URL")),
		ProjectID:  os.Getenv("CI_PROJECT_ID"),
		PipelineID: os.Getenv("CI_PIPELINE_ID"),
		Debug:      os.Getenv("DEBUG") == "true",
	}
}

// GetEndpoint returns the configured endpoint (host:port) stripped of any scheme
func (c *Config) GetEndpoint() string {
	ep := c.Endpoint
	if ep == "" {
		return getDefaultEndpoint(c.Protocol)
	}
	ep = strings.TrimPrefix(ep, "http://")
	ep = strings.TrimPrefix(ep, "https://")
	return ep
}

func getDefaultEndpoint(protocol string) string {
	switch protocol {
	case "http":
		return "localhost:4318"
	case "grpc":
		return "localhost:4317"
	case "stdout", "console":
		return "stdout"
	default:
		return "localhost:4318"
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
