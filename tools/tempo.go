package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	mcpgrafana "github.com/grafana/mcp-grafana"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// DefaultTempoTraceLimit is the default number of traces to return if not specified
	DefaultTempoTraceLimit = 20

	// MaxTempoTraceLimit is the maximum number of traces that can be requested
	MaxTempoTraceLimit = 100
)

// tempoClient represents a client for interacting with Tempo
type tempoClient struct {
	httpClient *http.Client
	baseURL    string
}

// TraceSearchResult represents a search result from Tempo
type TraceSearchResult struct {
	TraceID           string                 `json:"traceID"`
	RootServiceName   string                 `json:"rootServiceName"`
	RootTraceName     string                 `json:"rootTraceName"`
	StartTimeUnixNano string                 `json:"startTimeUnixNano"`
	DurationMs        int                    `json:"durationMs"`
	ServiceStats      map[string]interface{} `json:"serviceStats,omitempty"`
	SpanSet           interface{}            `json:"spanSet,omitempty"`
	SpanSets          []interface{}          `json:"spanSets,omitempty"`
}

// SearchTracesResponse represents the response from trace search
type SearchTracesResponse struct {
	Traces  []TraceSearchResult `json:"traces"`
	Metrics interface{}         `json:"metrics,omitempty"`
}

// TagValuesResponse represents the response from tag values endpoint
type TagValuesResponse struct {
	TagValues []string `json:"tagValues"`
}

// newTempoClient creates a new Tempo client
func newTempoClient(ctx context.Context, uid string) (*tempoClient, error) {
	// First check if the datasource exists
	_, err := getDatasourceByUID(ctx, GetDatasourceByUIDParams{UID: uid})
	if err != nil {
		return nil, err
	}

	cfg := mcpgrafana.GrafanaConfigFromContext(ctx)
	url := fmt.Sprintf("%s/api/datasources/proxy/uid/%s", strings.TrimRight(cfg.URL, "/"), uid)

	// Create custom transport with TLS configuration if available
	var transport http.RoundTripper = http.DefaultTransport
	if tlsConfig := cfg.TLSConfig; tlsConfig != nil {
		var err error
		transport, err = tlsConfig.HTTPTransport(transport.(*http.Transport))
		if err != nil {
			return nil, fmt.Errorf("failed to create custom transport: %w", err)
		}
	}

	client := &http.Client{
		Transport: &authRoundTripper{
			accessToken: cfg.AccessToken,
			idToken:     cfg.IDToken,
			apiKey:      cfg.APIKey,
			underlying:  transport,
		},
		Timeout: 30 * time.Second,
	}

	return &tempoClient{
		httpClient: client,
		baseURL:    url,
	}, nil
}

// buildURL constructs a full URL for a Tempo API endpoint
func (c *tempoClient) buildURL(urlPath string) string {
	fullURL := c.baseURL
	if !strings.HasSuffix(fullURL, "/") && !strings.HasPrefix(urlPath, "/") {
		fullURL += "/"
	} else if strings.HasSuffix(fullURL, "/") && strings.HasPrefix(urlPath, "/") {
		urlPath = strings.TrimPrefix(urlPath, "/")
	}
	return fullURL + urlPath
}

// makeRequest makes an HTTP request to the Tempo API
func (c *tempoClient) makeRequest(ctx context.Context, method, urlPath string, params url.Values) ([]byte, error) {
	fullURL := c.buildURL(urlPath)

	u, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	if params != nil {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Check for non-200 status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Tempo API returned status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the response body with a limit
	body := io.LimitReader(resp.Body, 1024*1024*48) // 48MB limit
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if len(bodyBytes) == 0 {
		return nil, fmt.Errorf("empty response from Tempo API")
	}

	return bodyBytes, nil
}

// SearchTempoTracesParams defines the parameters for searching traces
type SearchTempoTracesParams struct {
	DatasourceUID string            `json:"datasourceUid" jsonschema:"required,description=The UID of the datasource to query"`
	Query         string            `json:"query,omitempty" jsonschema:"description=The TraceQL query to execute. Example: {span.http.status_code=500} or {resource.service.name=\"checkout\"} or {.cluster=\"prod\"}"`
	Tags          map[string]string `json:"tags,omitempty" jsonschema:"description=Tags to filter traces by. This is an alternative to using a query string"`
	MinDuration   string            `json:"minDuration,omitempty" jsonschema:"description=Minimum duration of traces (e.g. '100ms'\\, '1s')"`
	MaxDuration   string            `json:"maxDuration,omitempty" jsonschema:"description=Maximum duration of traces (e.g. '100ms'\\, '1s')"`
	Limit         int               `json:"limit,omitempty" jsonschema:"description=The maximum number of traces to return (default: 20\\, max: 100)"`
	Start         int64             `json:"start,omitempty" jsonschema:"description=Start time in Unix nanoseconds. Defaults to 1 hour ago"`
	End           int64             `json:"end,omitempty" jsonschema:"description=End time in Unix nanoseconds. Defaults to now"`
}

// searchTempoTraces searches for traces in Tempo
func searchTempoTraces(ctx context.Context, args SearchTempoTracesParams) (*SearchTracesResponse, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	params := url.Values{}

	// Build query from tags if no query provided
	if args.Query == "" && len(args.Tags) > 0 {
		queryParts := []string{}
		for k, v := range args.Tags {
			queryParts = append(queryParts, fmt.Sprintf(`{.%s="%s"}`, k, v))
		}
		args.Query = strings.Join(queryParts, " && ")
	}

	if args.Query != "" {
		params.Add("q", args.Query)
	}

	if args.MinDuration != "" {
		params.Add("minDuration", args.MinDuration)
	}

	if args.MaxDuration != "" {
		params.Add("maxDuration", args.MaxDuration)
	}

	// Apply limit constraints
	limit := args.Limit
	if limit <= 0 {
		limit = DefaultTempoTraceLimit
	}
	if limit > MaxTempoTraceLimit {
		limit = MaxTempoTraceLimit
	}
	params.Add("limit", fmt.Sprintf("%d", limit))

	// Set time range - default to last hour if not specified
	if args.Start == 0 {
		args.Start = time.Now().Add(-1 * time.Hour).UnixNano()
	}
	if args.End == 0 {
		args.End = time.Now().UnixNano()
	}
	params.Add("start", fmt.Sprintf("%d", args.Start))
	params.Add("end", fmt.Sprintf("%d", args.End))

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/search", params)
	if err != nil {
		return nil, err
	}

	var response SearchTracesResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	return &response, nil
}

// SearchTempoTraces is a tool for searching traces in Tempo
var SearchTempoTraces = mcpgrafana.MustTool(
	"search_tempo_traces",
	"Search for traces in Tempo using TraceQL queries or tags. Returns a list of matching traces with metadata like trace ID, service name, duration, and start time. Supports filtering by duration and time range.",
	searchTempoTraces,
	mcp.WithTitleAnnotation("Search Tempo traces"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// GetTempoTraceParams defines the parameters for getting a specific trace
type GetTempoTraceParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the datasource to query"`
	TraceID       string `json:"traceId" jsonschema:"required,description=The trace ID to retrieve"`
}

// getTempoTrace retrieves a specific trace by ID
func getTempoTrace(ctx context.Context, args GetTempoTraceParams) (map[string]interface{}, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", fmt.Sprintf("/api/traces/%s", args.TraceID), nil)
	if err != nil {
		return nil, err
	}

	var trace map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &trace); err != nil {
		return nil, fmt.Errorf("unmarshalling trace: %w", err)
	}

	return trace, nil
}

// GetTempoTrace is a tool for retrieving a specific trace
var GetTempoTrace = mcpgrafana.MustTool(
	"get_tempo_trace",
	"Retrieve a specific trace from Tempo by its trace ID. Returns the complete trace data including all spans, their relationships, attributes, and timing information.",
	getTempoTrace,
	mcp.WithTitleAnnotation("Get Tempo trace"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ListTempoTagNamesParams defines the parameters for listing tag names
type ListTempoTagNamesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the datasource to query"`
	Scope         string `json:"scope,omitempty" jsonschema:"description=The scope of tags to retrieve: 'intrinsic'\\, 'span'\\, 'resource'\\, or leave empty for all"`
}

// listTempoTagNames lists all available tag names
func listTempoTagNames(ctx context.Context, args ListTempoTagNamesParams) ([]string, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	params := url.Values{}
	if args.Scope != "" {
		params.Add("scope", args.Scope)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/search/tags", params)
	if err != nil {
		return nil, err
	}

	var response struct {
		Scopes []struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		} `json:"scopes"`
	}

	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Flatten all tags from all scopes
	tagSet := make(map[string]bool)
	for _, scope := range response.Scopes {
		for _, tag := range scope.Tags {
			tagSet[tag] = true
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}

	return tags, nil
}

// ListTempoTagNames is a tool for listing available tag names
var ListTempoTagNames = mcpgrafana.MustTool(
	"list_tempo_tag_names",
	"List all available tag names in Tempo. Can be filtered by scope (intrinsic, span, resource). Returns a list of tag names that can be used for searching traces.",
	listTempoTagNames,
	mcp.WithTitleAnnotation("List Tempo tag names"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ListTempoTagValuesParams defines the parameters for listing tag values
type ListTempoTagValuesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the datasource to query"`
	TagName       string `json:"tagName" jsonschema:"required,description=The tag name to get values for"`
}

// listTempoTagValues lists all values for a specific tag
func listTempoTagValues(ctx context.Context, args ListTempoTagValuesParams) ([]string, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", fmt.Sprintf("/api/search/tag/%s/values", args.TagName), nil)
	if err != nil {
		return nil, err
	}

	var response TagValuesResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	return response.TagValues, nil
}

// ListTempoTagValues is a tool for listing tag values
var ListTempoTagValues = mcpgrafana.MustTool(
	"list_tempo_tag_values",
	"List all values for a specific tag name in Tempo. Useful for discovering what values are available for filtering traces.",
	listTempoTagValues,
	mcp.WithTitleAnnotation("List Tempo tag values"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// AddTempoTools registers all Tempo tools with the MCP server
func AddTempoTools(mcp *server.MCPServer) {
	SearchTempoTraces.Register(mcp)
	GetTempoTrace.Register(mcp)
	ListTempoTagNames.Register(mcp)
	ListTempoTagValues.Register(mcp)
}
