package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SonarrClient is a client for interacting with Sonarr API
type SonarrClient struct {
	*Client
}

// NewSonarrClient creates a new Sonarr API client
func NewSonarrClient(cfg ClientConfig) *SonarrClient {
	return &SonarrClient{
		Client: NewClient(cfg),
	}
}

// Series represents a TV series in Sonarr
type Series struct {
	ID            int       `json:"id"`
	Title         string    `json:"title"`
	SeasonCount   int       `json:"seasonCount"`
	Status        string    `json:"status"`
	Overview      string    `json:"overview"`
	Monitored     bool      `json:"monitored"`
	Added         time.Time `json:"added"`
	Path          string    `json:"path"`
	Statistics    Statistics `json:"statistics"`
	Seasons       []Season  `json:"seasons"`
}

// Season represents a season in a series
type Season struct {
	SeasonNumber int        `json:"seasonNumber"`
	Monitored    bool       `json:"monitored"`
	Statistics   Statistics `json:"statistics"`
}

// Statistics contains statistics for a series or season
type Statistics struct {
	EpisodeFileCount int     `json:"episodeFileCount"`
	EpisodeCount     int     `json:"episodeCount"`
	TotalEpisodeCount int    `json:"totalEpisodeCount"`
	SizeOnDisk       int64   `json:"sizeOnDisk"`
	PercentOfEpisodes float64 `json:"percentOfEpisodes"`
}

// Episode represents an episode in Sonarr
type Episode struct {
	ID              int       `json:"id"`
	SeriesID        int       `json:"seriesId"`
	EpisodeFileID   int       `json:"episodeFileId"`
	SeasonNumber    int       `json:"seasonNumber"`
	EpisodeNumber   int       `json:"episodeNumber"`
	Title           string    `json:"title"`
	AirDate         string    `json:"airDate"`
	AirDateUTC      time.Time `json:"airDateUtc"`
	HasFile         bool      `json:"hasFile"`
	Monitored       bool      `json:"monitored"`
}

// CommandBody represents a command request to Sonarr
type CommandBody struct {
	Name       string `json:"name"`
	SeriesID   *int   `json:"seriesId,omitempty"`
	SeasonNumber *int `json:"seasonNumber,omitempty"`
	EpisodeIDs []int  `json:"episodeIds,omitempty"`
}

// GetSeries retrieves a specific series by ID
func (c *SonarrClient) GetSeries(ctx context.Context, id int) (*Series, error) {
	endpoint := fmt.Sprintf("series/%d", id)

	var series Series
	if err := c.get(ctx, endpoint, &series); err != nil {
		return nil, fmt.Errorf("failed to get series %d: %w", id, err)
	}

	return &series, nil
}

// GetAllSeries retrieves all series from Sonarr
func (c *SonarrClient) GetAllSeries(ctx context.Context) ([]Series, error) {
	var series []Series
	if err := c.get(ctx, "series", &series); err != nil {
		return nil, fmt.Errorf("failed to get all series: %w", err)
	}

	return series, nil
}

// GetEpisodes retrieves all episodes for a series
func (c *SonarrClient) GetEpisodes(ctx context.Context, seriesID int) ([]Episode, error) {
	endpoint := fmt.Sprintf("episode?seriesId=%d", seriesID)

	var episodes []Episode
	if err := c.get(ctx, endpoint, &episodes); err != nil {
		return nil, fmt.Errorf("failed to get episodes for series %d: %w", seriesID, err)
	}

	return episodes, nil
}

// SearchEpisodes triggers a search for specific episodes
func (c *SonarrClient) SearchEpisodes(ctx context.Context, episodeIDs []int) error {
	// Properly marshal episode IDs as JSON array
	idsJSON, err := json.Marshal(episodeIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal episode IDs: %w", err)
	}
	body := strings.NewReader(fmt.Sprintf(`{"name":"EpisodeSearch","episodeIds":%s}`, string(idsJSON)))

	if err := c.post(ctx, "command", body); err != nil {
		return fmt.Errorf("failed to search episodes %v: %w", episodeIDs, err)
	}

	return nil
}

// SearchSeason triggers a search for an entire season
func (c *SonarrClient) SearchSeason(ctx context.Context, seriesID, seasonNum int) error {
	body := strings.NewReader(fmt.Sprintf(`{"name":"SeasonSearch","seriesId":%d,"seasonNumber":%d}`, seriesID, seasonNum))

	if err := c.post(ctx, "command", body); err != nil {
		return fmt.Errorf("failed to search season %d of series %d: %w", seasonNum, seriesID, err)
	}

	return nil
}

// DeleteSeries removes a series from Sonarr
func (c *SonarrClient) DeleteSeries(ctx context.Context, seriesID int, deleteFiles bool) error {
	endpoint := fmt.Sprintf("series/%d?deleteFiles=%t", seriesID, deleteFiles)

	if err := c.delete(ctx, endpoint); err != nil {
		return fmt.Errorf("failed to delete series %d: %w", seriesID, err)
	}

	return nil
}

// GetCutoffUnmet retrieves episodes that don't meet quality cutoff
func (c *SonarrClient) GetCutoffUnmet(ctx context.Context) ([]CutoffUnmetItem, error) {
	var cutoffResp CutoffUnmetResponse
	endpoint := "wanted/cutoff?page=1&pageSize=1000"

	if err := c.get(ctx, endpoint, &cutoffResp); err != nil {
		return nil, fmt.Errorf("failed to get cutoff unmet items: %w", err)
	}

	c.logger.DebugContext(ctx, "retrieved cutoff unmet episodes",
		"total_items", cutoffResp.TotalRecords,
		"page_size", cutoffResp.PageSize)

	return cutoffResp.Records, nil
}
