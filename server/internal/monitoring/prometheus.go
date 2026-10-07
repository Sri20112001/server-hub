// Package monitoring provides clients for Prometheus and Alertmanager.
// Server Hub acts as an authenticated API gateway — the browser never
// communicates with these systems directly.
package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// PrometheusClient is a thin HTTP client for the Prometheus HTTP API.
type PrometheusClient struct {
	baseURL string
	http    *http.Client
}

func NewPrometheusClient(baseURL string, timeoutSec int) *PrometheusClient {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	return &PrometheusClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

// Available reports whether a Prometheus URL is configured.
func (c *PrometheusClient) Available() bool { return c != nil && c.baseURL != "" }

// promResponse is the envelope returned by all Prometheus HTTP API endpoints.
type promResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	ErrorType string          `json:"errorType,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func (c *PrometheusClient) get(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	if !c.Available() {
		return nil, fmt.Errorf("prometheus not configured")
	}
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var pr promResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("invalid prometheus response: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus error (%s): %s", pr.ErrorType, pr.Error)
	}
	return pr.Data, nil
}

// Query executes an instant PromQL query (GET /api/v1/query).
func (c *PrometheusClient) Query(ctx context.Context, query string) (json.RawMessage, error) {
	return c.get(ctx, "/api/v1/query", url.Values{"query": {query}})
}

// QueryRange executes a range PromQL query (GET /api/v1/query_range).
func (c *PrometheusClient) QueryRange(ctx context.Context, query, start, end, step string) (json.RawMessage, error) {
	return c.get(ctx, "/api/v1/query_range", url.Values{
		"query": {query},
		"start": {start},
		"end":   {end},
		"step":  {step},
	})
}

// Targets returns the current scrape targets (GET /api/v1/targets).
func (c *PrometheusClient) Targets(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/api/v1/targets", nil)
}

// Rules returns alerting and recording rules (GET /api/v1/rules).
func (c *PrometheusClient) Rules(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/api/v1/rules", nil)
}

// Status checks Prometheus health (/-/healthy).
func (c *PrometheusClient) Status(ctx context.Context) (bool, string) {
	if !c.Available() {
		return false, "not configured"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/-/healthy", nil)
	if err != nil {
		return false, err.Error()
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err.Error()
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, "ok"
	}
	return false, fmt.Sprintf("http %d", resp.StatusCode)
}

// scalarFloat extracts a single float from a Prometheus instant vector result.
// Returns 0 and false if the result is empty or not a vector/scalar.
func ScalarFloat(data json.RawMessage) (float64, bool) {
	var result struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value [2]json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, false
	}
	if len(result.Result) == 0 {
		return 0, false
	}
	var v string
	if err := json.Unmarshal(result.Result[0].Value[1], &v); err != nil {
		return 0, false
	}
	var f float64
	if _, err := fmt.Sscanf(v, "%f", &f); err != nil {
		return 0, false
	}
	return f, true
}
