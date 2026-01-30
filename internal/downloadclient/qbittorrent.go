package downloadclient

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jmylchreest/go-declutarr/pkg/httpclient"
)

// QBittorrentClient implements the Client interface for qBittorrent WebUI API
type QBittorrentClient struct {
	baseURL  string
	username string
	password string
	http     *httpclient.Client
	logger   *slog.Logger
	sid      string // session cookie
}

// QBittorrentConfig holds configuration for creating a QBittorrentClient
type QBittorrentConfig struct {
	BaseURL  string
	Username string
	Password string
	Timeout  time.Duration
	SkipTLS  bool
	Logger   *slog.Logger
}

// qBitTorrentInfo represents the API response for torrent info
type qBitTorrentInfo struct {
	Hash           string  `json:"hash"`
	Name           string  `json:"name"`
	State          string  `json:"state"`
	Progress       float64 `json:"progress"`
	Size           int64   `json:"size"`
	Downloaded     int64   `json:"downloaded"`
	Uploaded       int64   `json:"uploaded"`
	Dlspeed        int64   `json:"dlspeed"`
	Upspeed        int64   `json:"upspeed"`
	Ratio          float64 `json:"ratio"`
	SeedingTime    int64   `json:"seeding_time"`
	AddedOn        int64   `json:"added_on"`
	CompletionOn   int64   `json:"completion_on"`
	SavePath       string  `json:"save_path"`
	Category       string  `json:"category"`
	Tags           string  `json:"tags"`
	TrackerHost    string  `json:"tracker"`
	MagnetURI      string  `json:"magnet_uri"`
	TotalSize      int64   `json:"total_size"`
	AmountLeft     int64   `json:"amount_left"`
	TimeActive     int64   `json:"time_active"`
	NumSeeds       int     `json:"num_seeds"`
	NumComplete    int     `json:"num_complete"`
	NumLeechs      int     `json:"num_leechs"`
	NumIncomplete  int     `json:"num_incomplete"`
	Priority       int     `json:"priority"`
	SequentialDL   bool    `json:"seq_dl"`
	FirstLastPiece bool    `json:"f_l_piece_prio"`
}

// NewQBittorrentClient creates a new qBittorrent WebUI API client
func NewQBittorrentClient(cfg QBittorrentConfig) (*QBittorrentClient, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
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

	client := &QBittorrentClient{
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		username: cfg.Username,
		password: cfg.Password,
		http:     httpclient.New(httpCfg),
		logger:   logger.With("service", "qbittorrent"),
	}

	return client, nil
}

// Login authenticates with the qBittorrent WebUI and retrieves session cookie
func (c *QBittorrentClient) Login(ctx context.Context) error {
	loginURL := c.baseURL + "/api/v2/auth/login"

	data := url.Values{}
	data.Set("username", c.username)
	data.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("execute login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}

	// Check for successful login
	if string(body) != "Ok." {
		return fmt.Errorf("login failed: %s", string(body))
	}

	// Extract SID cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "SID" {
			c.sid = cookie.Value
			c.logger.DebugContext(ctx, "authenticated with qbittorrent")
			return nil
		}
	}

	return fmt.Errorf("SID cookie not found in login response")
}

// Name returns the client name
func (c *QBittorrentClient) Name() string {
	return "qBittorrent"
}

// GetTorrents retrieves all torrents from qBittorrent
func (c *QBittorrentClient) GetTorrents(ctx context.Context) ([]Torrent, error) {
	if c.sid == "" {
		if err := c.Login(ctx); err != nil {
			return nil, fmt.Errorf("authentication required: %w", err)
		}
	}

	apiURL := c.baseURL + "/api/v2/torrents/info"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Cookie", fmt.Sprintf("SID=%s", c.sid))

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		// Session expired, re-login
		c.sid = ""
		return c.GetTorrents(ctx)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var qbitTorrents []qBitTorrentInfo
	if err := c.http.DecodeJSON(resp, &qbitTorrents); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	torrents := make([]Torrent, len(qbitTorrents))
	for i, qt := range qbitTorrents {
		torrents[i] = c.convertTorrent(&qt)
	}

	c.logger.DebugContext(ctx, "retrieved torrents", "count", len(torrents))
	return torrents, nil
}

// GetTorrent retrieves a specific torrent by hash
func (c *QBittorrentClient) GetTorrent(ctx context.Context, hash string) (*Torrent, error) {
	if c.sid == "" {
		if err := c.Login(ctx); err != nil {
			return nil, fmt.Errorf("authentication required: %w", err)
		}
	}

	apiURL := c.baseURL + "/api/v2/torrents/info?hashes=" + hash

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Cookie", fmt.Sprintf("SID=%s", c.sid))

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		// Session expired, re-login
		c.sid = ""
		return c.GetTorrent(ctx, hash)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var qbitTorrents []qBitTorrentInfo
	if err := c.http.DecodeJSON(resp, &qbitTorrents); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(qbitTorrents) == 0 {
		return nil, fmt.Errorf("torrent not found: %s", hash)
	}

	torrent := c.convertTorrent(&qbitTorrents[0])
	return &torrent, nil
}

// DeleteTorrent removes a torrent from qBittorrent
func (c *QBittorrentClient) DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error {
	if c.sid == "" {
		if err := c.Login(ctx); err != nil {
			return fmt.Errorf("authentication required: %w", err)
		}
	}

	apiURL := c.baseURL + "/api/v2/torrents/delete"

	data := url.Values{}
	data.Set("hashes", hash)
	if deleteFiles {
		data.Set("deleteFiles", "true")
	} else {
		data.Set("deleteFiles", "false")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", fmt.Sprintf("SID=%s", c.sid))

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		// Session expired, re-login
		c.sid = ""
		return c.DeleteTorrent(ctx, hash, deleteFiles)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	c.logger.DebugContext(ctx, "deleted torrent from qbittorrent", "hash", hash, "delete_files", deleteFiles)
	return nil
}

// PauseTorrent pauses a torrent in qBittorrent
func (c *QBittorrentClient) PauseTorrent(ctx context.Context, hash string) error {
	if c.sid == "" {
		if err := c.Login(ctx); err != nil {
			return fmt.Errorf("authentication required: %w", err)
		}
	}

	apiURL := c.baseURL + "/api/v2/torrents/pause"

	data := url.Values{}
	data.Set("hashes", hash)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", fmt.Sprintf("SID=%s", c.sid))

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		// Session expired, re-login
		c.sid = ""
		return c.PauseTorrent(ctx, hash)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	c.logger.DebugContext(ctx, "paused torrent", "hash", hash)
	return nil
}

// ResumeTorrent resumes a paused torrent in qBittorrent
func (c *QBittorrentClient) ResumeTorrent(ctx context.Context, hash string) error {
	if c.sid == "" {
		if err := c.Login(ctx); err != nil {
			return fmt.Errorf("authentication required: %w", err)
		}
	}

	apiURL := c.baseURL + "/api/v2/torrents/resume"

	data := url.Values{}
	data.Set("hashes", hash)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", fmt.Sprintf("SID=%s", c.sid))

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		// Session expired, re-login
		c.sid = ""
		return c.ResumeTorrent(ctx, hash)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	c.logger.DebugContext(ctx, "resumed torrent", "hash", hash)
	return nil
}

// convertTorrent converts qBittorrent API response to our Torrent type
func (c *QBittorrentClient) convertTorrent(qt *qBitTorrentInfo) Torrent {
	torrent := Torrent{
		Hash:          qt.Hash,
		Name:          qt.Name,
		State:         mapQBitState(qt.State),
		Progress:      qt.Progress,
		Size:          qt.Size,
		Downloaded:    qt.Downloaded,
		Uploaded:      qt.Uploaded,
		DownloadSpeed: qt.Dlspeed,
		UploadSpeed:   qt.Upspeed,
		Ratio:         qt.Ratio,
		SeedTime:      time.Duration(qt.SeedingTime) * time.Second,
		AddedOn:       time.Unix(qt.AddedOn, 0),
		SavePath:      qt.SavePath,
		Category:      qt.Category,
	}

	// Handle completion time
	if qt.CompletionOn > 0 {
		completedOn := time.Unix(qt.CompletionOn, 0)
		torrent.CompletedOn = &completedOn
	}

	// Parse tags (comma-separated string)
	if qt.Tags != "" {
		torrent.Tags = strings.Split(qt.Tags, ",")
		for i := range torrent.Tags {
			torrent.Tags[i] = strings.TrimSpace(torrent.Tags[i])
		}
	}

	// Add tracker if available
	if qt.TrackerHost != "" {
		torrent.Trackers = []string{qt.TrackerHost}
	}

	// Determine if private based on magnet URI or tracker
	// This is a simplified check - in reality, would need to check torrent metadata
	torrent.IsPrivate = false

	return torrent
}

// mapQBitState maps qBittorrent state strings to our TorrentState enum
func mapQBitState(state string) TorrentState {
	switch state {
	case "downloading", "metaDL", "forcedDL", "allocating":
		return StateDownloading
	case "uploading", "stalledUP", "forcedUP":
		return StateSeeding
	case "pausedDL", "pausedUP":
		return StatePaused
	case "stalledDL":
		return StateStalled
	case "error", "missingFiles":
		return StateError
	case "queuedDL", "queuedUP", "checkingDL", "checkingUP", "checkingResumeData":
		return StateQueued
	default:
		return StateQueued
	}
}
