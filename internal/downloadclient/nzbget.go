package downloadclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/jmylchreest/go-declutarr/pkg/httpclient"
)

// NZBGetClient implements the DownloadClient interface for NZBGet
type NZBGetClient struct {
	baseURL  string
	username string
	password string
	http     *httpclient.Client
	logger   *slog.Logger
}

// NZBGetConfig holds configuration for NZBGet client
type NZBGetConfig struct {
	BaseURL  string
	Username string
	Password string
	Timeout  time.Duration
	Logger   *slog.Logger
}

// NZBGetGroup represents a download group in NZBGet queue
type NZBGetGroup struct {
	NZBID           int    `json:"NZBID"`
	NZBName         string `json:"NZBName"`
	Status          string `json:"Status"`
	FileSizeMB      int    `json:"FileSizeMB"`
	RemainingSizeMB int    `json:"RemainingSizeMB"`
	Health          int    `json:"Health"` // permille (1000 = 100%)
	Category        string `json:"Category"`
}

// NZBGetHistoryItem represents a history item in NZBGet
type NZBGetHistoryItem struct {
	NZBID      int    `json:"NZBID"`
	NZBName    string `json:"NZBName"`
	Status     string `json:"Status"`
	Category   string `json:"Category"`
	FileSizeMB int    `json:"FileSizeMB"`
}

// rpcRequest represents a JSON-RPC request
type rpcRequest struct {
	Version string `json:"version"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

// rpcResponse represents a JSON-RPC response
type rpcResponse struct {
	Version string          `json:"version"`
	Result  json.RawMessage `json:"result"`
	Error   *string         `json:"error"`
}

// NewNZBGetClient creates a new NZBGet client
func NewNZBGetClient(cfg NZBGetConfig) *NZBGetClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	httpCfg := httpclient.Config{
		Timeout:         cfg.Timeout,
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
		SkipTLSVerify:   false,
	}

	return &NZBGetClient{
		baseURL:  cfg.BaseURL,
		username: cfg.Username,
		password: cfg.Password,
		http:     httpclient.New(httpCfg),
		logger:   cfg.Logger,
	}
}

// Name returns the name of the download client
func (c *NZBGetClient) Name() string {
	return "NZBGet"
}

// rpcCall performs a JSON-RPC call to NZBGet
func (c *NZBGetClient) rpcCall(ctx context.Context, method string, params []any, result any) error {
	// Build endpoint URL
	endpoint, err := url.JoinPath(c.baseURL, "jsonrpc")
	if err != nil {
		return fmt.Errorf("failed to build endpoint URL: %w", err)
	}

	// Create RPC request
	req := rpcRequest{
		Version: "1.1",
		Method:  method,
		Params:  params,
	}

	// Marshal request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal RPC request: %w", err)
	}

	c.logger.Debug("NZBGet RPC call",
		"method", method,
		"endpoint", endpoint,
	)

	// Create HTTP request with Basic Auth
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Add Basic Auth
	if c.username != "" || c.password != "" {
		httpReq.SetBasicAuth(c.username, c.password)
	}

	// Execute the request
	resp, err := c.http.Do(ctx, httpReq)
	if err != nil {
		return fmt.Errorf("failed to execute RPC request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("RPC call failed with HTTP %d", resp.StatusCode)
	}

	// Decode RPC response
	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("failed to decode RPC response: %w", err)
	}

	// Check for RPC error
	if rpcResp.Error != nil {
		return fmt.Errorf("RPC error: %s", *rpcResp.Error)
	}

	// Unmarshal result into target
	if result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("failed to unmarshal RPC result: %w", err)
		}
	}

	return nil
}

// GetQueue retrieves the current download queue from NZBGet
func (c *NZBGetClient) GetQueue(ctx context.Context) ([]NZBGetGroup, error) {
	var groups []NZBGetGroup
	err := c.rpcCall(ctx, "listgroups", []any{}, &groups)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue: %w", err)
	}

	c.logger.Debug("Retrieved NZBGet queue",
		"count", len(groups),
	)

	return groups, nil
}

// GetHistory retrieves the download history from NZBGet
func (c *NZBGetClient) GetHistory(ctx context.Context) ([]NZBGetHistoryItem, error) {
	var history []NZBGetHistoryItem
	err := c.rpcCall(ctx, "history", []any{}, &history)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	c.logger.Debug("Retrieved NZBGet history",
		"count", len(history),
	)

	return history, nil
}

// DeleteItem deletes an item from the queue by NZB ID
func (c *NZBGetClient) DeleteItem(ctx context.Context, nzbID int) error {
	// editqueue with GroupFinalDelete action
	params := []any{"GroupFinalDelete", 0, "", []int{nzbID}}

	var success bool
	err := c.rpcCall(ctx, "editqueue", params, &success)
	if err != nil {
		return fmt.Errorf("failed to delete item %d: %w", nzbID, err)
	}

	if !success {
		return fmt.Errorf("delete operation failed for item %d", nzbID)
	}

	c.logger.Info("Deleted NZBGet item",
		"nzbID", nzbID,
	)

	return nil
}

// PauseItem pauses a download group by NZB ID
func (c *NZBGetClient) PauseItem(ctx context.Context, nzbID int) error {
	// editqueue with GroupPause action
	params := []any{"GroupPause", 0, "", []int{nzbID}}

	var success bool
	err := c.rpcCall(ctx, "editqueue", params, &success)
	if err != nil {
		return fmt.Errorf("failed to pause item %d: %w", nzbID, err)
	}

	if !success {
		return fmt.Errorf("pause operation failed for item %d", nzbID)
	}

	c.logger.Info("Paused NZBGet item",
		"nzbID", nzbID,
	)

	return nil
}

// ResumeItem resumes a paused download group by NZB ID
func (c *NZBGetClient) ResumeItem(ctx context.Context, nzbID int) error {
	// editqueue with GroupResume action
	params := []any{"GroupResume", 0, "", []int{nzbID}}

	var success bool
	err := c.rpcCall(ctx, "editqueue", params, &success)
	if err != nil {
		return fmt.Errorf("failed to resume item %d: %w", nzbID, err)
	}

	if !success {
		return fmt.Errorf("resume operation failed for item %d", nzbID)
	}

	c.logger.Info("Resumed NZBGet item",
		"nzbID", nzbID,
	)

	return nil
}
