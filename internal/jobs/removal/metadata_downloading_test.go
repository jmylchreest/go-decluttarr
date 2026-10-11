package removal

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmylchreest/go-decluttarr/internal/arrapi"
	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/downloadclient"
	"github.com/jmylchreest/go-decluttarr/internal/jobs"
)

func metadataItem() arrapi.QueueItem {
	id := 123
	return arrapi.QueueItem{ID: 1, MovieID: &id, DownloadID: "hash", DownloadClient: "qbit", Protocol: "torrent", Status: "queued", TrackedDownloadState: "downloading", TrackedDownloadStatus: "ok", ErrorMessage: "qBittorrent is downloading metadata"}
}
func TestMetadataDownloadingMatchesIssue2(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	j := NewMetadataDownloadingJob("metadata", &config.JobConfig{}, &config.JobDefaultsConfig{MaxStrikes: 3}, nil, logger, false)
	if !j.matches(metadataItem()) {
		t.Fatal("issue #2 reproduction not detected")
	}
	old := NewMetadataMissingJob("old", &config.JobConfig{}, &config.JobDefaultsConfig{MaxStrikes: 3}, nil, logger, false)
	if len(old.FindAffected([]arrapi.QueueItem{metadataItem()})) != 0 {
		t.Fatal("legacy library-matching semantics changed")
	}
	for _, tc := range []struct {
		name string
		edit func(*arrapi.QueueItem)
		want bool
	}{
		{"healthy", func(i *arrapi.QueueItem) { i.ErrorMessage = "" }, false},
		{"title only", func(i *arrapi.QueueItem) {
			i.ErrorMessage = ""
			i.StatusMessages = []arrapi.StatusMessage{{Title: "DOWNLOADING METADATA"}}
		}, true},
		{"nested message", func(i *arrapi.QueueItem) {
			i.ErrorMessage = ""
			i.StatusMessages = []arrapi.StatusMessage{{Messages: []string{"downloading metadata"}}}
		}, true},
		{"complete stale message", func(i *arrapi.QueueItem) { i.Status = "completed" }, false},
		{"importing stale message", func(i *arrapi.QueueItem) { i.TrackedDownloadState = "importing" }, false},
		{"usenet", func(i *arrapi.QueueItem) { i.Protocol = "usenet" }, false},
		{"no hash", func(i *arrapi.QueueItem) { i.DownloadID = "" }, false},
		{"no queue identity", func(i *arrapi.QueueItem) { i.ID = 0 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := metadataItem()
			tc.edit(&i)
			if got := j.matches(i); got != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
	custom := NewMetadataDownloadingJob("metadata", &config.JobConfig{MessagePatterns: []string{" Fetching Magnet "}}, &config.JobDefaultsConfig{MaxStrikes: 3}, nil, logger, false)
	i := metadataItem()
	i.ErrorMessage = "Fetching Magnet"
	if !custom.matches(i) || custom.matches(metadataItem()) {
		t.Fatal("custom literal patterns not honored")
	}
}

type metadataClient struct {
	safetyClient
	tagged int
}

func (c *metadataClient) AddTags(context.Context, string, []string) error { c.tagged++; return nil }

func TestMetadataDownloadingSafetyAndStrikes(t *testing.T) {
	for _, mode := range []string{"remove", "dry", "protected", "shelfarr", "private", "ignored", "unknown", "offline", "delete failure", "tag", "recovered"} {
		t.Run(mode, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			cfg := &config.Config{General: config.GeneralConfig{PublicTrackerHandling: "remove", PrivateTrackerHandling: "keep", ProtectedTag: "Keep", ObsoleteTag: "Obsolete"}}
			if mode == "ignored" {
				cfg.General.IgnoreDownloadClients = []string{"qbit"}
			}
			if mode == "shelfarr" {
				cfg.Instances.Shelfarr = []config.ShelfarrConfig{{ProtectedCategories: []string{"books"}}}
			}
			if mode == "tag" {
				cfg.General.PublicTrackerHandling = "obsolete_tag"
			}
			dc := &metadataClient{safetyClient: safetyClient{torrents: []downloadclient.Torrent{{Hash: "hash", Category: "books"}}, private: mode == "private", lookupErr: mode == "unknown"}}
			if mode == "protected" {
				dc.torrents[0].Tags = []string{"Keep"}
			}
			m := jobs.NewManager(cfg, logger, "")
			defer m.Close()
			m.RegisterDownloadClient("qbit", dc)
			item := metadataItem()
			deleted := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "offline" {
					w.WriteHeader(503)
					return
				}
				if r.Method == "DELETE" {
					deleted++
					q := r.URL.Query()
					if q.Get("removeFromClient") != "true" || q.Get("blocklist") != "true" || q.Get("skipRedownload") != "false" {
						t.Error("wrong deletion semantics")
					}
					if mode == "delete failure" {
						w.WriteHeader(503)
						return
					}
					w.WriteHeader(204)
					return
				}
				// Duplicate episode records must not accelerate strikes.
				json.NewEncoder(w).Encode(arrapi.QueueResponse{TotalRecords: 2, Records: []arrapi.QueueItem{item, item}})
			}))
			defer server.Close()
			m.RegisterArrClient("radarr", arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL}))
			j := NewMetadataDownloadingJob("metadata", &config.JobConfig{Enabled: true}, &config.JobDefaultsConfig{MaxStrikes: 3}, m, logger, mode == "dry")
			// Unrelated rules cannot advance this job's threshold.
			m.GetStrikesHandler().Add("hash", "other", "")
			for cycle := 1; cycle <= 3; cycle++ {
				if mode == "recovered" && cycle == 2 {
					item.ErrorMessage = ""
				}
				if mode == "recovered" && cycle == 3 {
					item = metadataItem()
				}
				err := j.Run(context.Background())
				wantErr := mode == "offline" || (mode == "delete failure" && cycle == 3)
				if (err != nil) != wantErr {
					t.Fatalf("cycle %d err=%v", cycle, err)
				}
				if cycle < 3 && deleted != 0 {
					t.Fatal("deleted before threshold")
				}
			}
			wantDeleted := 0
			if mode == "remove" || mode == "delete failure" {
				wantDeleted = 1
			}
			if deleted != wantDeleted {
				t.Fatalf("deleted=%d want=%d", deleted, wantDeleted)
			}
			if mode == "tag" && dc.tagged != 1 {
				t.Fatal("tag policy not applied")
			}
			if mode == "recovered" && m.GetStrikesHandler().Get(j.strikeKey("radarr", item)) != 1 {
				t.Fatal("recovery did not reset own strikes")
			}
			if m.GetStrikesHandler().Get("hash") != 1 {
				t.Fatal("modified another job's strikes")
			}
			if mode == "delete failure" && m.GetStrikesHandler().Get(j.strikeKey("radarr", item)) != 3 {
				t.Fatal("failed deletion lost strikes")
			}
			if mode == "dry" && j.Stats().Removed != 0 {
				t.Fatal("dry run reported a deletion")
			}
		})
	}
}
