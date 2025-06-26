//go:build cloud
// +build cloud

// This file contains cloud integration tests that run against a dedicated test instance
// with Tempo configured. These tests expect a Tempo instance with some trace data
// and will skip if the required environment variables are not set.

package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudTempoTools(t *testing.T) {
	ctx := createCloudTestContext(t, "Tempo", "GRAFANA_URL", "GRAFANA_API_KEY")

	t.Run("list tempo tag names", func(t *testing.T) {
		result, err := listTempoTagNames(ctx, ListTempoTagNamesParams{
			DatasourceUID: "tempo",
		})
		require.NoError(t, err, "Should not error when listing tag names")
		assert.NotEmpty(t, result, "Should have at least some tag names")

		// Common tags that should exist in most Tempo instances
		commonTags := []string{"service.name", "name", "kind"}
		foundCommonTag := false
		for _, tag := range result {
			for _, common := range commonTags {
				if tag == common {
					foundCommonTag = true
					break
				}
			}
		}
		assert.True(t, foundCommonTag, "Should have at least one common tag like service.name, name, or kind")
	})

	t.Run("list tempo tag values", func(t *testing.T) {
		// First get available tags
		tags, err := listTempoTagNames(ctx, ListTempoTagNamesParams{
			DatasourceUID: "tempo",
		})
		require.NoError(t, err, "Should not error when listing tag names")
		require.NotEmpty(t, tags, "Should have at least one tag")

		// Try to find a common tag
		var tagToTest string
		for _, tag := range tags {
			if tag == "service.name" || tag == "name" || tag == "kind" {
				tagToTest = tag
				break
			}
		}

		if tagToTest != "" {
			result, err := listTempoTagValues(ctx, ListTempoTagValuesParams{
				DatasourceUID: "tempo",
				TagName:       tagToTest,
			})
			require.NoError(t, err, "Should not error when listing tag values")
			assert.NotEmpty(t, result, "Should have at least one value for tag %s", tagToTest)
		}
	})

	t.Run("search tempo traces", func(t *testing.T) {
		// Search for recent traces
		endTime := time.Now().UnixNano()
		startTime := time.Now().Add(-24 * time.Hour).UnixNano()

		result, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			Limit:         5,
			Start:         startTime,
			End:           endTime,
		})
		require.NoError(t, err, "Should not error when searching traces")
		assert.NotNil(t, result, "Should return a result")

		// If we have traces, verify their structure
		if len(result.Traces) > 0 {
			trace := result.Traces[0]
			assert.NotEmpty(t, trace.TraceID, "Trace should have an ID")
			assert.NotEmpty(t, trace.StartTimeUnixNano, "Trace should have a start time")
		}
	})

	t.Run("search tempo traces with query", func(t *testing.T) {
		// Try to search with a basic query
		result, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			Query:         `{}`, // Empty query to get all traces
			Limit:         3,
		})
		require.NoError(t, err, "Should not error when searching traces with query")
		assert.NotNil(t, result, "Should return a result")
	})

	t.Run("get tempo trace", func(t *testing.T) {
		// First search for a trace to get its ID
		searchResult, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			Limit:         1,
		})
		require.NoError(t, err, "Should not error when searching for traces")

		if len(searchResult.Traces) > 0 {
			traceID := searchResult.Traces[0].TraceID

			// Get the full trace
			trace, err := getTempoTrace(ctx, GetTempoTraceParams{
				DatasourceUID: "tempo",
				TraceID:       traceID,
			})
			require.NoError(t, err, "Should not error when getting trace")
			assert.NotNil(t, trace, "Should return trace data")

			// Verify basic structure
			if traceIDField, ok := trace["traceID"]; ok {
				assert.Equal(t, traceID, traceIDField, "Returned trace ID should match requested ID")
			}
		} else {
			t.Skip("No traces available to test with")
		}
	})

	t.Run("search tempo traces with tags", func(t *testing.T) {
		// First get available service names
		serviceNames, err := listTempoTagValues(ctx, ListTempoTagValuesParams{
			DatasourceUID: "tempo",
			TagName:       "service.name",
		})

		if err == nil && len(serviceNames) > 0 {
			// Search for traces from a specific service
			result, err := searchTempoTraces(ctx, SearchTempoTracesParams{
				DatasourceUID: "tempo",
				Tags: map[string]string{
					"service.name": serviceNames[0],
				},
				Limit: 3,
			})
			require.NoError(t, err, "Should not error when searching traces with tags")
			assert.NotNil(t, result, "Should return a result")
		} else {
			t.Skip("No service names available to test tag search")
		}
	})

	t.Run("search tempo traces with duration filter", func(t *testing.T) {
		// Search for traces with specific duration constraints
		result, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			MinDuration:   "1ms",
			MaxDuration:   "10s",
			Limit:         5,
		})
		require.NoError(t, err, "Should not error when searching traces with duration filter")
		assert.NotNil(t, result, "Should return a result")

		// Verify traces meet duration criteria if any are returned
		for _, trace := range result.Traces {
			assert.GreaterOrEqual(t, trace.DurationMs, 1, "Trace duration should be at least 1ms")
			assert.LessOrEqual(t, trace.DurationMs, 10000, "Trace duration should be at most 10000ms (10s)")
		}
	})

	t.Run("list tempo tag names with scope", func(t *testing.T) {
		// Test different scopes
		scopes := []string{"span", "resource", "intrinsic"}

		for _, scope := range scopes {
			result, err := listTempoTagNames(ctx, ListTempoTagNamesParams{
				DatasourceUID: "tempo",
				Scope:         scope,
			})
			// Don't fail the test if a scope isn't supported
			if err == nil {
				assert.NotNil(t, result, "Should return tags for scope: %s", scope)
			}
		}
	})
}
