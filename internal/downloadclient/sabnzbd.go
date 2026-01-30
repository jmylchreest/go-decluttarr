package downloadclient

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/jmylchreest/go-declutarr/pkg/httpclient"
)

// SABnzbdClient implements the Client interface for SABnzbd
type SABnzbdClient struct {
	baseURL string
	apiKey  string
	http    *httpclient.Client
	logger  *slog.Logger
}

// SABnzbdConfig holds configuration for SABnzbd client
type SABnzbdConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	Logger  *slog.Logger
}

// SABnzbdSlot represents an item in the SABnzbd queue
type SABnzbdSlot struct {
	NzoID      string  `json:"nzo_id"`
	Filename   string  `json:"filename"`
	Status     string  `json:"status"`
	Size       string  `json:"size"`
	Sizeleft   string  `json:"sizeleft"`
	Percentage string  `json:"percentage"`
	Category   string  `json:"cat"`
	MBLeft     float64 `json:"mbleft"`
	MB         float64 `json:"mb"`
}

// SABnzbdHistorySlot represents an item in the SABnzbd history
type SABnzbdHistorySlot struct {
	NzoID    string `json:"nzo_id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Bytes    string `json:"bytes"`
	Category string `json:"category"`
	Storage  string `json:"storage"`
}

// SABnzbdQueueResponse represents the queue API response
type SABnzbdQueueResponse struct {
	Queue struct {
		Slots []SABnzbdSlot `json:"slots"`
	} `json:"queue"`
}

// SABnzbdHistoryResponse represents the history API response
type SABnzbdHistoryResponse struct {
	History struct {
		Slots []SABnzbdHistorySlot `json:"slots"`
	} `json:"history"`
}

// NewSABnzbdClient creates a new SABnzbd client
func NewSABnzbdClient(cfg SABnzbdConfig) *SABnzbdClient {
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

	return &SABnzbdClient{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		http:    httpclient.New(httpCfg),
		logger:  cfg.Logger,
	}
}

// Name returns the client name
func (c *SABnzbdClient) Name() string {
	return "SABnzbd"
}

// buildURL constructs API URL with mode and apikey parameters
func (c *SABnzbdClient) buildURL(mode string, extraParams map[string]string) string {
	params := url.Values{}
	params.Set("mode", mode)
	params.Set("apikey", c.apiKey)
	params.Set("output", "json")

	for k, v := range extraParams {
		params.Set(k, v)
	}

	return fmt.Sprintf("%s/api?%s", c.baseURL, params.Encode())
}

// GetQueue retrieves the current download queue
func (c *SABnzbdClient) GetQueue(ctx context.Context) ([]SABnzbdSlot, error) {
	apiURL := c.buildURL("queue", nil)

	c.logger.Debug("fetching SABnzbd queue", "url", apiURL)

	resp, err := c.http.Get(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch queue: %w", err)
	}

	var queueResp SABnzbdQueueResponse
	if err := c.http.DecodeJSON(resp, &queueResp); err != nil {
		return nil, fmt.Errorf("failed to decode queue response: %w", err)
	}

	c.logger.Debug("fetched sabnzbd queue", "count", len(queueResp.Queue.Slots))
	return queueResp.Queue.Slots, nil
}

// GetHistory retrieves the download history
func (c *SABnzbdClient) GetHistory(ctx context.Context) ([]SABnzbdHistorySlot, error) {
	apiURL := c.buildURL("history", nil)

	c.logger.Debug("fetching SABnzbd history", "url", apiURL)

	resp, err := c.http.Get(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch history: %w", err)
	}

	var historyResp SABnzbdHistoryResponse
	if err := c.http.DecodeJSON(resp, &historyResp); err != nil {
		return nil, fmt.Errorf("failed to decode history response: %w", err)
	}

	c.logger.Debug("fetched sabnzbd history", "count", len(historyResp.History.Slots))
	return historyResp.History.Slots, nil
}

// DeleteSlot deletes an item from the queue by NZO ID
func (c *SABnzbdClient) DeleteSlot(ctx context.Context, nzoID string) error {
	params := map[string]string{
		"name":  "delete",
		"value": nzoID,
	}
	apiURL := c.buildURL("queue", params)

	c.logger.Debug("deleting SABnzbd slot", "nzo_id", nzoID)

	resp, err := c.http.Get(ctx, apiURL)
	if err != nil {
		return fmt.Errorf("failed to delete slot: %w", err)
	}
	_ = resp.Body.Close()

	c.logger.Debug("deleted sabnzbd slot", "nzo_id", nzoID)
	return nil
}

// PauseSlot pauses a download by NZO ID
func (c *SABnzbdClient) PauseSlot(ctx context.Context, nzoID string) error {
	params := map[string]string{
		"name":  "pause",
		"value": nzoID,
	}
	apiURL := c.buildURL("queue", params)

	c.logger.Debug("pausing SABnzbd slot", "nzo_id", nzoID)

	resp, err := c.http.Get(ctx, apiURL)
	if err != nil {
		return fmt.Errorf("failed to pause slot: %w", err)
	}
	_ = resp.Body.Close()

	c.logger.Debug("paused sabnzbd slot", "nzo_id", nzoID)
	return nil
}

// ResumeSlot resumes a paused download by NZO ID
func (c *SABnzbdClient) ResumeSlot(ctx context.Context, nzoID string) error {
	params := map[string]string{
		"name":  "resume",
		"value": nzoID,
	}
	apiURL := c.buildURL("queue", params)

	c.logger.Debug("resuming SABnzbd slot", "nzo_id", nzoID)

	resp, err := c.http.Get(ctx, apiURL)
	if err != nil {
		return fmt.Errorf("failed to resume slot: %w", err)
	}
	_ = resp.Body.Close()

	c.logger.Debug("resumed sabnzbd slot", "nzo_id", nzoID)
	return nil
}

// GetTorrents adapts SABnzbd queue to Client interface (returns queue items as Torrent-like objects)
func (c *SABnzbdClient) GetTorrents(ctx context.Context) ([]Torrent, error) {
	slots, err := c.GetQueue(ctx)
	if err != nil {
		return nil, err
	}

	torrents := make([]Torrent, 0, len(slots))
	for _, slot := range slots {
		torrent, err := c.slotToTorrent(slot)
		if err != nil {
			c.logger.Warn("failed to convert slot to torrent", "nzo_id", slot.NzoID, "error", err)
			continue
		}
		torrents = append(torrents, torrent)
	}

	return torrents, nil
}

// GetTorrent retrieves a single item by NZO ID
func (c *SABnzbdClient) GetTorrent(ctx context.Context, nzoID string) (*Torrent, error) {
	slots, err := c.GetQueue(ctx)
	if err != nil {
		return nil, err
	}

	for _, slot := range slots {
		if slot.NzoID == nzoID {
			torrent, err := c.slotToTorrent(slot)
			if err != nil {
				return nil, err
			}
			return &torrent, nil
		}
	}

	return nil, fmt.Errorf("slot not found: %s", nzoID)
}

// DeleteTorrent deletes an item (adapter for Client interface)
func (c *SABnzbdClient) DeleteTorrent(ctx context.Context, nzoID string, deleteFiles bool) error {
	return c.DeleteSlot(ctx, nzoID)
}

// PauseTorrent pauses an item (adapter for Client interface)
func (c *SABnzbdClient) PauseTorrent(ctx context.Context, nzoID string) error {
	return c.PauseSlot(ctx, nzoID)
}

// ResumeTorrent resumes an item (adapter for Client interface)
func (c *SABnzbdClient) ResumeTorrent(ctx context.Context, nzoID string) error {
	return c.ResumeSlot(ctx, nzoID)
}

// slotToTorrent converts SABnzbd slot to Torrent structure
func (c *SABnzbdClient) slotToTorrent(slot SABnzbdSlot) (Torrent, error) {
	var state TorrentState
	switch slot.Status {
	case "Paused":
		state = StatePaused
	case "Downloading", "Fetching":
		state = StateDownloading
	case "Queued":
		state = StateQueued
	default:
		state = StateDownloading
	}

	progress := 0.0
	if slot.Percentage != "" {
		if p, err := strconv.ParseFloat(slot.Percentage, 64); err == nil {
			progress = p / 100.0
		}
	}

	// Calculate size in bytes (MB to bytes)
	size := int64(slot.MB * 1024 * 1024)
	downloaded := size - int64(slot.MBLeft*1024*1024)

	return Torrent{
		Hash:       slot.NzoID,
		Name:       slot.Filename,
		State:      state,
		Progress:   progress,
		Size:       size,
		Downloaded: downloaded,
		Category:   slot.Category,
		Tags:       []string{},
		Trackers:   []string{},
		IsPrivate:  false,
	}, nil
}

// Close closes the HTTP client
func (c *SABnzbdClient) Close() {
	c.http.Close()
}
