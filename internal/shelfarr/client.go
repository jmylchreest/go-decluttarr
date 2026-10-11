// Package shelfarr provides read-only access to Shelfarr requests. Requests are
// never translated into arr queue entries or used as proof of torrent ownership.
package shelfarr

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jmylchreest/go-decluttarr/pkg/httpclient"
)

const RequestLimit = 100 // Upstream caps the list endpoint; no pagination is exposed.

type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	SkipTLS bool
}

type Client struct {
	baseURL string
	apiKey  string
	http    *httpclient.Client
}

type Request struct {
	ID              int    `json:"id"`
	Status          string `json:"status"`
	AttentionNeeded bool   `json:"attention_needed"`
}

// Snapshot describes only the most recent requests visible to the supplied token.
// It must never be interpreted as a complete inventory of owned downloads.
type Snapshot struct {
	Observed        int            `json:"observed"`
	AtLimit         bool           `json:"at_limit"`
	AttentionNeeded int            `json:"attention_needed"`
	StatusCounts    map[string]int `json:"status_counts"`
}

func NewClient(cfg Config) *Client {
	httpCfg := httpclient.DefaultConfig()
	if cfg.Timeout > 0 {
		httpCfg.Timeout = cfg.Timeout
	}
	httpCfg.SkipTLSVerify = cfg.SkipTLS
	return &Client{baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: cfg.APIKey, http: httpclient.New(httpCfg)}
}

func (c *Client) GetSnapshot(ctx context.Context) (*Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/requests?limit=%d", c.baseURL, RequestLimit), nil)
	if err != nil {
		return nil, fmt.Errorf("create Shelfarr request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("fetch Shelfarr requests: %w", err)
	}
	// A missing/null requests field is not an empty successful response.
	var payload struct {
		Requests *[]Request `json:"requests"`
	}
	if err := c.http.DecodeJSON(resp, &payload); err != nil {
		return nil, fmt.Errorf("decode Shelfarr requests: %w", err)
	}
	if payload.Requests == nil {
		return nil, fmt.Errorf("Shelfarr response is missing requests")
	}
	snapshot := &Snapshot{Observed: len(*payload.Requests), AtLimit: len(*payload.Requests) >= RequestLimit, StatusCounts: make(map[string]int)}
	for _, request := range *payload.Requests {
		if request.ID <= 0 || request.Status == "" {
			return nil, fmt.Errorf("invalid Shelfarr request record")
		}
		snapshot.StatusCounts[request.Status]++ // Preserve unfamiliar states as diagnostic information.
		if request.AttentionNeeded {
			snapshot.AttentionNeeded++
		}
	}
	return snapshot, nil
}

func (c *Client) Close() { c.http.Close() }
