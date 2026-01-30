package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jmylchreest/go-declutarr/pkg/httpclient"
)

// Client provides base functionality for all *arr API clients
type Client struct {
	name       string
	baseURL    string
	apiKey     string
	apiVersion string
	http       *httpclient.Client
	logger     *slog.Logger
}

// ClientConfig holds configuration for creating a Client
type ClientConfig struct {
	Name       string
	BaseURL    string
	APIKey     string
	APIVersion string // "v1" for Lidarr/Readarr, "v3" for Sonarr/Radarr
	Timeout    time.Duration
	SkipTLS    bool
	Logger     *slog.Logger
}

// NewClient creates a new *arr API client
func NewClient(cfg ClientConfig) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = "v3" // Default to v3 for Sonarr/Radarr
	}

	httpCfg := httpclient.Config{
		Timeout:         cfg.Timeout,
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
		SkipTLSVerify:   cfg.SkipTLS,
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		name:       cfg.Name,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		apiVersion: cfg.APIVersion,
		http:       httpclient.New(httpCfg),
		logger:     logger.With("service", cfg.Name),
	}
}

// GetQueue retrieves all items in the download queue
func (c *Client) GetQueue(ctx context.Context) ([]QueueItem, error) {
	var queueResp QueueResponse
	path := fmt.Sprintf("/api/%s/queue?page=1&pageSize=1000", c.apiVersion)
	if err := c.request(ctx, http.MethodGet, path, nil, &queueResp); err != nil {
		return nil, fmt.Errorf("get queue: %w", err)
	}

	c.logger.DebugContext(ctx, "retrieved queue",
		"total_items", queueResp.TotalRecords,
		"page_size", queueResp.PageSize)

	return queueResp.Records, nil
}

// DeleteQueueItem removes an item from the queue
func (c *Client) DeleteQueueItem(ctx context.Context, id int, opts DeleteOptions) error {
	path := fmt.Sprintf("/api/%s/queue/%d", c.apiVersion, id)

	// Build query parameters
	params := url.Values{}
	if opts.RemoveFromClient {
		params.Set("removeFromClient", "true")
	}
	if opts.Blocklist {
		params.Set("blocklist", "true")
	}
	if opts.SkipRedownload {
		params.Set("skipRedownload", "true")
	}

	if len(params) > 0 {
		path = path + "?" + params.Encode()
	}

	if err := c.request(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("delete queue item %d: %w", id, err)
	}

	c.logger.DebugContext(ctx, "deleted queue item from arr",
		"id", id,
		"remove_from_client", opts.RemoveFromClient,
		"blocklist", opts.Blocklist)

	return nil
}

// GetSystemStatus retrieves the system status information
func (c *Client) GetSystemStatus(ctx context.Context) (*SystemStatus, error) {
	var status SystemStatus
	path := fmt.Sprintf("/api/%s/system/status", c.apiVersion)
	if err := c.request(ctx, http.MethodGet, path, nil, &status); err != nil {
		return nil, fmt.Errorf("get system status: %w", err)
	}

	c.logger.DebugContext(ctx, "retrieved system status",
		"app", status.AppName,
		"version", status.Version,
		"instance", status.InstanceName)

	return &status, nil
}

// apiURL constructs a full API URL from a path
func (c *Client) apiURL(path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + path
}

// request executes an API request with proper authentication and error handling
func (c *Client) request(ctx context.Context, method, path string, body, result any) error {
	fullURL := c.apiURL(path)

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = strings.NewReader(string(jsonData))
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Add API key authentication
	req.Header.Set("X-Api-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	c.logger.DebugContext(ctx, "API request",
		"method", method,
		"url", fullURL)

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.logger.ErrorContext(ctx, "API error response",
			"status", resp.StatusCode,
			"body", string(bodyBytes))
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// If result is nil, we don't need to decode (e.g., DELETE requests)
	if result == nil {
		return nil
	}

	// Decode response body
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// GetMonitoredStatus retrieves the monitored status for an entity
func (c *Client) GetMonitoredStatus(ctx context.Context, entityType string, id int) (bool, error) {
	var result struct {
		Monitored bool `json:"monitored"`
	}

	endpoint := fmt.Sprintf("/api/%s/%s/%d", c.apiVersion, entityType, id)
	if err := c.request(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
		return false, fmt.Errorf("get %s %d monitored status: %w", entityType, id, err)
	}

	return result.Monitored, nil
}

// Close closes the underlying HTTP client connections
func (c *Client) Close() {
	c.http.Close()
}

// get is a convenience method for GET requests
func (c *Client) get(ctx context.Context, path string, result any) error {
	return c.request(ctx, http.MethodGet, fmt.Sprintf("/api/%s/%s", c.apiVersion, path), nil, result)
}

// post is a convenience method for POST requests
func (c *Client) post(ctx context.Context, path string, body io.Reader) error {
	// Convert io.Reader to the format expected by request
	var bodyData any
	if body != nil {
		// Read the body content
		bodyBytes, err := io.ReadAll(body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		// Unmarshal to get the actual data structure
		if err := json.Unmarshal(bodyBytes, &bodyData); err != nil {
			return fmt.Errorf("unmarshal body: %w", err)
		}
	}
	return c.request(ctx, http.MethodPost, fmt.Sprintf("/api/%s/%s", c.apiVersion, path), bodyData, nil)
}

// delete is a convenience method for DELETE requests
func (c *Client) delete(ctx context.Context, path string) error {
	return c.request(ctx, http.MethodDelete, fmt.Sprintf("/api/%s/%s", c.apiVersion, path), nil, nil)
}
