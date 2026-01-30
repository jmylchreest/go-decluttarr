package arrapi

import "time"

// QueueItem represents a queue item in the *arr API
type QueueItem struct {
	ID                      int             `json:"id"`
	Title                   string          `json:"title"`
	Status                  string          `json:"status"`
	TrackedDownloadStatus   string          `json:"trackedDownloadStatus"`
	TrackedDownloadState    string          `json:"trackedDownloadState"`
	StatusMessages          []StatusMessage `json:"statusMessages"`
	ErrorMessage            string          `json:"errorMessage"`
	DownloadID              string          `json:"downloadId"`
	Protocol                string          `json:"protocol"`
	DownloadClient          string          `json:"downloadClient"`
	Indexer                 string          `json:"indexer"`
	OutputPath              string          `json:"outputPath"`
	Size                    int64           `json:"size"`
	Sizeleft                int64           `json:"sizeleft"`
	Added                   time.Time       `json:"added"`
	EstimatedCompletionTime *time.Time      `json:"estimatedCompletionTime"`

	// Sonarr-specific
	SeriesID     *int `json:"seriesId,omitempty"`
	EpisodeID    *int `json:"episodeId,omitempty"`
	SeasonNumber *int `json:"seasonNumber,omitempty"`

	// Radarr-specific
	MovieID *int `json:"movieId,omitempty"`

	// Lidarr-specific
	ArtistID *int `json:"artistId,omitempty"`
	AlbumID  *int `json:"albumId,omitempty"`

	// Readarr-specific
	AuthorID *int `json:"authorId,omitempty"`
	BookID   *int `json:"bookId,omitempty"`
}

// StatusMessage represents a status message in a queue item
type StatusMessage struct {
	Title    string   `json:"title"`
	Messages []string `json:"messages"`
}

// QueueResponse represents the paginated response from the queue API
type QueueResponse struct {
	Page         int         `json:"page"`
	PageSize     int         `json:"pageSize"`
	TotalRecords int         `json:"totalRecords"`
	Records      []QueueItem `json:"records"`
}

// DeleteOptions represents options for deleting a queue item
type DeleteOptions struct {
	RemoveFromClient bool `json:"removeFromClient"`
	Blocklist        bool `json:"blocklist"`
	SkipRedownload   bool `json:"skipRedownload"`
}
