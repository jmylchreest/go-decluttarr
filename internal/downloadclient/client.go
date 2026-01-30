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
