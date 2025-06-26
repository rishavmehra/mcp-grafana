//go:build integration

package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTempoTools(t *testing.T) {
	// Wait a bit for k6-tracing to generate some traces
	time.Sleep(5 * time.Second)

	t.Run("list tempo tag names", func(t *testing.T) {
		ctx := newTestContext()
		result, err := listTempoTagNames(ctx, ListTempoTagNamesParams{
			DatasourceUID: "tempo",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, result, "Should have at least some tag names")

		// Check for some expected tags
		expectedTags := []string{"service.name", "span.kind", "status.code"}
		for _, expected := range expectedTags {
			assert.Contains(t, result, expected, "Should contain expected tag: %s", expected)
		}
	})

	t.Run("list tempo tag values", func(t *testing.T) {
		ctx := newTestContext()
		result, err := listTempoTagValues(ctx, ListTempoTagValuesParams{
			DatasourceUID: "tempo",
			TagName:       "service.name",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, result, "Should have at least one service name value")
	})

	t.Run("search tempo traces", func(t *testing.T) {
		ctx := newTestContext()

		// Search for all traces in the last hour
		result, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			Limit:         10,
		})
		require.NoError(t, err)
		assert.NotNil(t, result, "Should return a result")

		if len(result.Traces) > 0 {
			// Verify trace structure
			trace := result.Traces[0]
			assert.NotEmpty(t, trace.TraceID, "Trace should have an ID")
			assert.NotEmpty(t, trace.RootServiceName, "Trace should have a root service name")
			assert.NotEmpty(t, trace.StartTimeUnixNano, "Trace should have a start time")
			assert.Greater(t, trace.DurationMs, 0, "Trace should have a positive duration")
		}
	})

	t.Run("search tempo traces with query", func(t *testing.T) {
		ctx := newTestContext()

		// Search for traces with a specific attribute
		result, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			Query:         `{span.kind="server"}`,
			Limit:         5,
		})
		require.NoError(t, err)
		assert.NotNil(t, result, "Should return a result")
	})

	t.Run("search tempo traces with tags", func(t *testing.T) {
		ctx := newTestContext()

		// First get a service name to search for
		tagValues, err := listTempoTagValues(ctx, ListTempoTagValuesParams{
			DatasourceUID: "tempo",
			TagName:       "service.name",
		})
		require.NoError(t, err)
		require.NotEmpty(t, tagValues, "Need at least one service name")

		// Search for traces from that service
		result, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			Tags: map[string]string{
				"service.name": tagValues[0],
			},
			Limit: 5,
		})
		require.NoError(t, err)
		assert.NotNil(t, result, "Should return a result")
	})

	t.Run("search tempo traces with duration filter", func(t *testing.T) {
		ctx := newTestContext()

		// Search for traces longer than 10ms
		result, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			MinDuration:   "10ms",
			Limit:         5,
		})
		require.NoError(t, err)
		assert.NotNil(t, result, "Should return a result")

		// Verify all returned traces meet the duration criteria
		for _, trace := range result.Traces {
			assert.GreaterOrEqual(t, trace.DurationMs, 10, "Trace duration should be at least 10ms")
		}
	})

	t.Run("get tempo trace", func(t *testing.T) {
		ctx := newTestContext()

		// First search for a trace to get its ID
		searchResult, err := searchTempoTraces(ctx, SearchTempoTracesParams{
			DatasourceUID: "tempo",
			Limit:         1,
		})
		require.NoError(t, err)
		require.NotEmpty(t, searchResult.Traces, "Need at least one trace to test")

		traceID := searchResult.Traces[0].TraceID

		// Get the full trace
		trace, err := getTempoTrace(ctx, GetTempoTraceParams{
			DatasourceUID: "tempo",
			TraceID:       traceID,
		})
		require.NoError(t, err)
		assert.NotNil(t, trace, "Should return trace data")

		// Verify the trace has expected structure
		assert.NotEmpty(t, trace["traceID"], "Trace should have an ID")

		// Check for batches/spans
		batches, ok := trace["batches"].([]interface{})
		if ok && len(batches) > 0 {
			batch := batches[0].(map[string]interface{})

			// Check for spans in the batch
			if scopeSpans, ok := batch["scopeSpans"].([]interface{}); ok && len(scopeSpans) > 0 {
				scopeSpan := scopeSpans[0].(map[string]interface{})
				if spans, ok := scopeSpan["spans"].([]interface{}); ok {
					assert.NotEmpty(t, spans, "Trace should have spans")
				}
			}
		}
	})

	t.Run("list tempo tag names with scope", func(t *testing.T) {
		ctx := newTestContext()

		// Test different scopes
		scopes := []string{"span", "resource", "intrinsic"}
		for _, scope := range scopes {
			result, err := listTempoTagNames(ctx, ListTempoTagNamesParams{
				DatasourceUID: "tempo",
				Scope:         scope,
			})
			require.NoError(t, err)
			assert.NotNil(t, result, "Should return tags for scope: %s", scope)
		}
	})
}
