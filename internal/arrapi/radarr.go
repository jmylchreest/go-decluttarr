package arrapi

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RadarrClient is a client for interacting with Radarr API
type RadarrClient struct {
	*Client
}

// NewRadarrClient creates a new Radarr API client
func NewRadarrClient(cfg ClientConfig) *RadarrClient {
	return &RadarrClient{
		Client: NewClient(cfg),
	}
}

// Movie represents a movie in Radarr
type Movie struct {
	ID                  int        `json:"id"`
	Title               string     `json:"title"`
	OriginalTitle       string     `json:"originalTitle"`
	Year                int        `json:"year"`
	Status              string     `json:"status"`
	Overview            string     `json:"overview"`
	Path                string     `json:"path"`
	Monitored           bool       `json:"monitored"`
	Added               time.Time  `json:"added"`
	HasFile             bool       `json:"hasFile"`
	SizeOnDisk          int64      `json:"sizeOnDisk"`
	Runtime             int        `json:"runtime"`
	MinimumAvailability string     `json:"minimumAvailability"`
	IsAvailable         bool       `json:"isAvailable"`
	LastSearchTime      *time.Time `json:"lastSearchTime,omitempty"`
}

// GetMovie retrieves a specific movie by ID
func (c *RadarrClient) GetMovie(ctx context.Context, id int) (*Movie, error) {
	endpoint := fmt.Sprintf("movie/%d", id)

	var movie Movie
	if err := c.get(ctx, endpoint, &movie); err != nil {
		return nil, fmt.Errorf("failed to get movie %d: %w", id, err)
	}

	return &movie, nil
}

// GetAllMovies retrieves all movies from Radarr
func (c *RadarrClient) GetAllMovies(ctx context.Context) ([]Movie, error) {
	var movies []Movie
	if err := c.get(ctx, "movie", &movies); err != nil {
		return nil, fmt.Errorf("failed to get all movies: %w", err)
	}

	return movies, nil
}

// SearchMovie triggers a search for a specific movie
func (c *RadarrClient) SearchMovie(ctx context.Context, movieID int) error {
	body := strings.NewReader(fmt.Sprintf(`{"name":"MoviesSearch","movieIds":[%d]}`, movieID))

	if err := c.post(ctx, "command", body); err != nil {
		return fmt.Errorf("failed to search movie %d: %w", movieID, err)
	}

	return nil
}

// DeleteMovie removes a movie from Radarr
func (c *RadarrClient) DeleteMovie(ctx context.Context, movieID int, deleteFiles bool) error {
	endpoint := fmt.Sprintf("movie/%d?deleteFiles=%t", movieID, deleteFiles)

	if err := c.delete(ctx, endpoint); err != nil {
		return fmt.Errorf("failed to delete movie %d: %w", movieID, err)
	}

	return nil
}

// GetCutoffUnmet retrieves movies that don't meet quality cutoff
func (c *RadarrClient) GetCutoffUnmet(ctx context.Context) ([]CutoffUnmetItem, error) {
	var cutoffResp CutoffUnmetResponse
	endpoint := "wanted/cutoff?page=1&pageSize=1000"

	if err := c.get(ctx, endpoint, &cutoffResp); err != nil {
		return nil, fmt.Errorf("failed to get cutoff unmet items: %w", err)
	}

	c.logger.DebugContext(ctx, "retrieved cutoff unmet movies",
		"total_items", cutoffResp.TotalRecords,
		"page_size", cutoffResp.PageSize)

	return cutoffResp.Records, nil
}
