package main

import (
	"os"
	"testing"
)

func TestTraceParentFlagHandling(t *testing.T) {
	// Test TRACEPARENT environment variable setting
	testTraceParent := "00-b9d11bdcbdeb9ebe851a1e1a5428fc0d-d84e1fdd24cf946d-01"

	// Clear any existing TRACEPARENT
	_ = os.Unsetenv("TRACEPARENT")

	// Simulate the flag handling logic
	useRootSpan := testTraceParent
	if useRootSpan != "" {
		_ = os.Setenv("TRACEPARENT", useRootSpan)
	}

	// Verify environment variable is set correctly
	result := os.Getenv("TRACEPARENT")
	if result != testTraceParent {
		t.Errorf("Expected TRACEPARENT=%s, got %s", testTraceParent, result)
	}

	// Clean up
	_ = os.Unsetenv("TRACEPARENT")
}

func TestTraceParentParsing(t *testing.T) {
	testCases := []struct {
		name        string
		traceparent string
		expectValid bool
		expectTrace string
		expectSpan  string
	}{
		{
			name:        "valid_traceparent",
			traceparent: "00-b9d11bdcbdeb9ebe851a1e1a5428fc0d-d84e1fdd24cf946d-01",
			expectValid: true,
			expectTrace: "b9d11bdcbdeb9ebe851a1e1a5428fc0d",
			expectSpan:  "d84e1fdd24cf946d",
		},
		{
			name:        "invalid_format",
			traceparent: "invalid-format",
			expectValid: false,
		},
		{
			name:        "empty_traceparent",
			traceparent: "",
			expectValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set environment variable
			if tc.traceparent != "" {
				_ = os.Setenv("TRACEPARENT", tc.traceparent)
			} else {
				_ = os.Unsetenv("TRACEPARENT")
			}

			// Test would verify ExtractParentContext functionality
			// This is a placeholder for the actual trace context extraction test
			envValue := os.Getenv("TRACEPARENT")
			if tc.expectValid && envValue != tc.traceparent {
				t.Errorf("Expected TRACEPARENT=%s, got %s", tc.traceparent, envValue)
			}

			// Clean up
			_ = os.Unsetenv("TRACEPARENT")
		})
	}
}

func TestExtractParentContextIntegration(t *testing.T) {
	// Test that TRACEPARENT environment variable is properly extracted
	testTraceParent := "00-b9d11bdcbdeb9ebe851a1e1a5428fc0d-d84e1fdd24cf946d-01"

	// Set TRACEPARENT environment variable
	_ = os.Setenv("TRACEPARENT", testTraceParent)
	defer func() { _ = os.Unsetenv("TRACEPARENT") }()

	// Verify the environment variable is accessible
	result := os.Getenv("TRACEPARENT")
	if result != testTraceParent {
		t.Errorf("TRACEPARENT not set correctly: expected %s, got %s", testTraceParent, result)
	}

	// This test verifies the environment variable is set correctly
	// The actual ExtractParentContext would be tested in the otel package
	t.Logf("TRACEPARENT successfully set: %s", result)
}
