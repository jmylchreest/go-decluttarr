package downloadclient

import (
	"context"
	"time"
)

// Client defines the interface for interacting with download clients
type Client interface {
	Name() string
	GetTorrents(ctx context.Context) ([]Torrent, error)
	GetTorrent(ctx context.Context, hash string) (*Torrent, error)
	DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error
	PauseTorrent(ctx context.Context, hash string) error
	ResumeTorrent(ctx context.Context, hash string) error
	GetTorrentProperties(ctx context.Context, hash string) (*TorrentProperties, error)
	AddTags(ctx context.Context, hash string, tags []string) error
	IsPrivateTracker(ctx context.Context, hash string) (bool, error)
}

// Torrent represents a torrent in a download client
type Torrent struct {
	Hash          string
	Name          string
	State         TorrentState
	Progress      float64 // 0.0 to 1.0
	Size          int64
	Downloaded    int64
	Uploaded      int64
	DownloadSpeed int64
	UploadSpeed   int64
	Ratio         float64
	SeedTime      time.Duration
	AddedOn       time.Time
	CompletedOn   *time.Time
	SavePath      string
	Category      string
	Tags          []string
	Trackers      []string
	IsPrivate     bool
}

// TorrentState represents the state of a torrent
type TorrentState string

const (
	StateDownloading TorrentState = "downloading"
	StateSeeding     TorrentState = "seeding"
	StatePaused      TorrentState = "paused"
	StateStalled     TorrentState = "stalled"
	StateError       TorrentState = "error"
	StateQueued      TorrentState = "queued"
)

// TorrentProperties represents detailed properties of a torrent
type TorrentProperties struct {
	IsPrivate         bool    `json:"is_private"`
	RatioLimit        float64 `json:"ratio_limit"`
	SeedingTimeLimit  int64   `json:"seeding_time_limit"`
	AdditionDate      int64   `json:"addition_date"`
	CompletionDate    int64   `json:"completion_date"`
	CreatedBy         string  `json:"created_by"`
	CreationDate      int64   `json:"creation_date"`
	Comment           string  `json:"comment"`
	TotalSize         int64   `json:"total_size"`
	PieceSize         int64   `json:"piece_size"`
	PiecesHave        int     `json:"pieces_have"`
	PiecesNum         int     `json:"pieces_num"`
	Reannounce        int64   `json:"reannounce"`
	SavePath          string  `json:"save_path"`
	SeedingTime       int64   `json:"seeding_time"`
	Seeds             int     `json:"seeds"`
	SeedsTotal        int     `json:"seeds_total"`
	ShareRatio        float64 `json:"share_ratio"`
	TimeElapsed       int64   `json:"time_elapsed"`
	TotalDownloaded   int64   `json:"total_downloaded"`
	TotalUploaded     int64   `json:"total_uploaded"`
	UploadLimit       int64   `json:"up_limit"`
	DownloadLimit     int64   `json:"dl_limit"`
	NbConnections     int     `json:"nb_connections"`
	NbConnectionsLimit int    `json:"nb_connections_limit"`
}

// TrackerInfo represents information about a torrent tracker
type TrackerInfo struct {
	URL           string `json:"url"`
	Status        int    `json:"status"`
	Tier          int    `json:"tier"`
	NumPeers      int    `json:"num_peers"`
	NumSeeds      int    `json:"num_seeds"`
	NumLeeches    int    `json:"num_leeches"`
	NumDownloaded int    `json:"num_downloaded"`
	Msg           string `json:"msg"`
}

// HasTag checks if a torrent has a specific tag
func HasTag(torrent *Torrent, tag string) bool {
	for _, t := range torrent.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
