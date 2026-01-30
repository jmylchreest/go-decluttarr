package arrapi

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LidarrClient provides API access to Lidarr
type LidarrClient struct {
	*Client
}

// NewLidarrClient creates a new Lidarr API client
func NewLidarrClient(cfg ClientConfig) *LidarrClient {
	return &LidarrClient{
		Client: NewClient(cfg),
	}
}

// Artist represents a Lidarr artist
type Artist struct {
	ID                int       `json:"id"`
	ForeignArtistID   string    `json:"foreignArtistId"`
	ArtistName        string    `json:"artistName"`
	CleanName         string    `json:"cleanName"`
	Monitored         bool      `json:"monitored"`
	Status            string    `json:"status"`
	Overview          string    `json:"overview"`
	ArtistType        string    `json:"artistType"`
	Disambiguation    string    `json:"disambiguation"`
	Path              string    `json:"path"`
	QualityProfileID  int       `json:"qualityProfileId"`
	MetadataProfileID int       `json:"metadataProfileId"`
	Added             time.Time `json:"added"`
	Statistics        *struct {
		AlbumCount      int     `json:"albumCount"`
		TrackFileCount  int     `json:"trackFileCount"`
		TrackCount      int     `json:"trackCount"`
		TotalTrackCount int     `json:"totalTrackCount"`
		SizeOnDisk      int64   `json:"sizeOnDisk"`
		PercentOfTracks float64 `json:"percentOfTracks"`
	} `json:"statistics,omitempty"`
}

// Album represents a Lidarr album
type Album struct {
	ID             int       `json:"id"`
	ForeignAlbumID string    `json:"foreignAlbumId"`
	Title          string    `json:"title"`
	CleanTitle     string    `json:"cleanTitle"`
	Monitored      bool      `json:"monitored"`
	ArtistID       int       `json:"artistId"`
	ReleaseDate    time.Time `json:"releaseDate"`
	AlbumType      string    `json:"albumType"`
	Overview       string    `json:"overview"`
	Genres         []string  `json:"genres"`
	Statistics     *struct {
		TrackFileCount  int     `json:"trackFileCount"`
		TrackCount      int     `json:"trackCount"`
		TotalTrackCount int     `json:"totalTrackCount"`
		SizeOnDisk      int64   `json:"sizeOnDisk"`
		PercentOfTracks float64 `json:"percentOfTracks"`
	} `json:"statistics,omitempty"`
}

// GetArtist retrieves an artist by ID
func (c *LidarrClient) GetArtist(ctx context.Context, id int) (*Artist, error) {
	endpoint := fmt.Sprintf("artist/%d", id)

	var artist Artist
	if err := c.get(ctx, endpoint, &artist); err != nil {
		return nil, fmt.Errorf("failed to get artist %d: %w", id, err)
	}

	return &artist, nil
}

// GetAlbum retrieves an album by ID
func (c *LidarrClient) GetAlbum(ctx context.Context, id int) (*Album, error) {
	endpoint := fmt.Sprintf("album/%d", id)

	var album Album
	if err := c.get(ctx, endpoint, &album); err != nil {
		return nil, fmt.Errorf("failed to get album %d: %w", id, err)
	}

	return &album, nil
}

// SearchAlbum triggers a search for an album by ID
func (c *LidarrClient) SearchAlbum(ctx context.Context, albumID int) error {
	body := strings.NewReader(fmt.Sprintf(`{"name":"AlbumSearch","albumIds":[%d]}`, albumID))

	if err := c.post(ctx, "command", body); err != nil {
		return fmt.Errorf("failed to search album %d: %w", albumID, err)
	}

	return nil
}
