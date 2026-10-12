package removal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmylchreest/go-decluttarr/internal/arrapi"
	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/downloadclient"
	"github.com/jmylchreest/go-decluttarr/internal/jobs"
	"github.com/jmylchreest/go-decluttarr/internal/shelfarr"
)

type safetyClient struct {
	downloadclient.Client
	torrents  []downloadclient.Torrent
	deleted   []string
	private   bool
	lookupErr bool
}

func (c *safetyClient) Name() string { return "qBittorrent" }
func (c *safetyClient) GetTorrents(context.Context) ([]downloadclient.Torrent, error) {
	return c.torrents, nil
}
func (c *safetyClient) GetTorrent(_ context.Context, hash string) (*downloadclient.Torrent, error) {
	for i := range c.torrents {
		if c.torrents[i].Hash == hash {
			return &c.torrents[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (c *safetyClient) IsPrivateTracker(context.Context, string) (bool, error) {
	if c.lookupErr {
		return false, fmt.Errorf("offline")
	}
	return c.private, nil
}
func (c *safetyClient) DeleteTorrent(_ context.Context, hash string, files bool) error {
	if files {
		return fmt.Errorf("unexpected file deletion")
	}
	c.deleted = append(c.deleted, hash)
	return nil
}
func (c *safetyClient) GetTorrentProperties(context.Context, string) (*downloadclient.TorrentProperties, error) {
	return &downloadclient.TorrentProperties{RatioLimit: 1}, nil
}

func TestOrphanSafety(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		scoped, offline, noArr, dry bool
		wantErr                     bool
		wantDeleted                 int
	}{
		{name: "Shelfarr excluded and hash case normalized", scoped: true, wantDeleted: 1},
		{name: "incomplete queues fail closed", scoped: true, offline: true, wantErr: true},
		{name: "no scope fails closed", wantErr: true},
		{name: "no arr fails closed", scoped: true, noArr: true, wantErr: true},
		{name: "dry run", scoped: true, dry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			cfg := &config.Config{General: config.GeneralConfig{PublicTrackerHandling: "remove", PrivateTrackerHandling: "keep", ProtectedTag: "Keep"}}
			m := jobs.NewManager(cfg, logger, "")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.offline {
					w.WriteHeader(503)
					return
				}
				fmt.Fprint(w, `{"totalRecords":1,"records":[{"downloadId":"ABC"}]}`)
			}))
			defer server.Close()
			if !tc.noArr {
				m.RegisterArrClient("sonarr", arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL}))
			}
			defer m.Close()
			dc := &safetyClient{torrents: []downloadclient.Torrent{
				{Hash: "shelfarr", Category: "shelfarr"}, {Hash: "abc", Category: "tv"},
				{Hash: "orphan", Category: "tv"}, {Hash: "protected", Category: "tv", Tags: []string{"Keep"}},
				{Hash: "uncategorized"},
			}}
			m.RegisterDownloadClient("qbit", dc)
			jobCfg := &config.JobConfig{Enabled: true}
			if tc.scoped {
				jobCfg.TargetCategories = []string{"tv"}
			}
			j := NewOrphansJob("remove_orphans", jobCfg, &config.JobDefaultsConfig{MaxStrikes: 1}, m, logger, tc.dry)
			err := j.Run(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if len(dc.deleted) != tc.wantDeleted {
				t.Fatalf("deleted=%v", dc.deleted)
			}
			if m.GetStrikesHandler().Get("shelfarr") != 0 || m.GetStrikesHandler().Get("abc") != 0 {
				t.Fatal("struck owned downloads")
			}
			if tc.wantErr && m.GetStrikesHandler().Count() != 0 {
				t.Fatal("strikes on incomplete evidence")
			}
		})
	}
}

func TestRemovalProtection(t *testing.T) {
	for _, tc := range []struct {
		name, handling                string
		private, lookupErr, protected bool
		want                          string
	}{
		{name: "private keep", handling: "keep", private: true, want: "skip"},
		{name: "private skip", handling: "skip", private: true, want: "skip"},
		{name: "unknown policy", handling: "oops", private: true, want: "skip"},
		{name: "tracker unavailable", handling: "remove", lookupErr: true, want: "skip"},
		{name: "protected", handling: "remove", protected: true, want: "skip"},
		{name: "explicit remove", handling: "remove", private: true, want: "remove"},
		{name: "tag", handling: "obsolete_tag", private: true, want: "tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			m := jobs.NewManager(&config.Config{General: config.GeneralConfig{PublicTrackerHandling: "remove", PrivateTrackerHandling: tc.handling, ProtectedTag: "Keep"}}, logger, "")
			torrent := downloadclient.Torrent{Hash: "a", Category: "tv", Progress: 1, State: downloadclient.StatePaused, Ratio: 2, SeedTime: time.Hour}
			if tc.protected {
				torrent.Tags = []string{"Keep"}
			}
			dc := &safetyClient{torrents: []downloadclient.Torrent{torrent}, private: tc.private, lookupErr: tc.lookupErr}
			m.RegisterDownloadClient("qbit", dc)
			if got := m.GetRemovalAction(context.Background(), "a"); got != tc.want {
				t.Fatalf("action=%s", got)
			}
			if got := m.GetRemovalAction(context.Background(), "missing"); got != "skip" {
				t.Fatalf("missing action=%s", got)
			}
			j := NewDoneSeedingJob("done", &config.RemoveDoneSeedingConfig{TargetCategories: []string{"tv"}}, m, logger, false)
			if err := j.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			wantDeleted := 0
			if tc.want == "remove" {
				wantDeleted = 1
			}
			if len(dc.deleted) != wantDeleted {
				t.Fatalf("deleted=%v", dc.deleted)
			}
		})
	}
}

func TestDoneSeedingRequiresScope(t *testing.T) {
	j := NewDoneSeedingJob("done", &config.RemoveDoneSeedingConfig{}, nil, slog.Default(), false)
	if err := j.Run(context.Background()); err == nil {
		t.Fatal("unscoped job accepted")
	}
}

func TestUnmonitoredCachesPerEntityAndDoesNotCacheErrors(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{}`)
			return
		}
		fmt.Fprint(w, `{"monitored":true}`)
	}))
	defer server.Close()
	client := arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL})
	defer client.Close()
	j := &UnmonitoredJob{logger: slog.Default()}
	id := 1
	item := &arrapi.QueueItem{SeriesID: &id}
	cache := make(map[string]bool)
	if _, err := j.checkUnmonitored(context.Background(), client, "Sonarr", item, cache); err == nil {
		t.Fatal("missing monitored flag accepted")
	}
	for i := 0; i < 10; i++ {
		unmonitored, err := j.checkUnmonitored(context.Background(), client, "Sonarr", item, cache)
		if err != nil || unmonitored {
			t.Fatalf("unmonitored=%v err=%v", unmonitored, err)
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d, wanted one error plus one successful lookup", calls)
	}
	// A subsequent job run must revalidate the entity.
	_, err := j.checkUnmonitored(context.Background(), client, "Sonarr", item, make(map[string]bool))
	if err != nil || calls != 3 {
		t.Fatalf("next run calls=%d err=%v", calls, err)
	}
}

func TestShelfarrProtectionSurvivesOutageAndOverlappingScopes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		General:   config.GeneralConfig{PublicTrackerHandling: "remove"},
		Instances: config.InstancesConfig{Shelfarr: []config.ShelfarrConfig{{Name: "shelfarr", ProtectedCategories: []string{"books"}}}},
	}
	m := jobs.NewManager(cfg, logger, "")
	defer m.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/requests" {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"totalRecords":0,"records":[]}`)
	}))
	defer server.Close()
	m.RegisterArrClient("bookshelf", arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL, APIVersion: "v1", AppType: "bookshelf"}))
	m.RegisterShelfarrClient("shelfarr", shelfarr.NewClient(shelfarr.Config{BaseURL: server.URL}))
	dc := &safetyClient{torrents: []downloadclient.Torrent{{Hash: "protected", Category: "books", Progress: 1, State: downloadclient.StatePaused, Ratio: 2, SeedTime: time.Hour}}}
	m.RegisterDownloadClient("qbit", dc)
	m.RegisterJob(NewOrphansJob("orphans", &config.JobConfig{Enabled: true, TargetCategories: []string{"books"}}, &config.JobDefaultsConfig{MaxStrikes: 1}, m, logger, false))
	m.RegisterJob(NewDoneSeedingJob("done", &config.RemoveDoneSeedingConfig{Enabled: true, TargetCategories: []string{"books"}}, m, logger, false))
	if err := m.RunAll(context.Background()); err == nil {
		t.Fatal("monitoring outage not reported")
	}
	if len(dc.deleted) != 0 || m.GetStrikesHandler().Count() != 0 {
		t.Fatalf("protected downloads affected: %v", dc.deleted)
	}
	if got := m.GetRemovalAction(context.Background(), "protected"); got != "skip" {
		t.Fatalf("queue-job policy=%s", got)
	}
}

func TestSlowSkipsMissingAddedTimestamp(t *testing.T) {
	j := NewSlowDownloadJob("slow", &config.JobConfig{}, &config.JobDefaultsConfig{MinDownloadSpeed: 1000}, nil, slog.Default(), false)
	items := []arrapi.QueueItem{{Status: "downloading", Size: 10000, Sizeleft: 9999}}
	if got := j.FindAffected(items); len(got) != 0 {
		t.Fatal("missing timestamp treated as slow")
	}
	items[0].Added = time.Now().Add(-time.Hour)
	if got := j.FindAffected(items); len(got) != 1 {
		t.Fatal("known slow download not detected")
	}
}

func TestNewIntegrationMonitoredMappings(t *testing.T) {
	for _, kind := range []string{"sportarr", "bookshelf"} {
		t.Run(kind, func(t *testing.T) {
			expected := "/api/events/99"
			if kind == "bookshelf" {
				expected = "/api/v1/author/99"
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != expected {
					t.Errorf("path=%s want=%s", r.URL.Path, expected)
				}
				fmt.Fprint(w, `{"monitored":false}`)
			}))
			defer server.Close()
			client := arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL, AppType: kind, APIVersion: "v1"})
			defer client.Close()
			id := 99
			item := &arrapi.QueueItem{EpisodeID: &id, AuthorID: &id}
			j := &UnmonitoredJob{logger: slog.Default()}
			got, err := j.checkUnmonitored(context.Background(), client, "Readarr", item, make(map[string]bool))
			if err != nil || !got || calls != 1 {
				t.Fatalf("unmonitored=%v calls=%d err=%v", got, calls, err)
			}
			zero := 0
			item.EpisodeID = &zero
			item.AuthorID = &zero
			got, err = j.checkUnmonitored(context.Background(), client, "Readarr", item, make(map[string]bool))
			if err != nil || got || calls != 1 {
				t.Fatal("unidentified pending import should be skipped")
			}
		})
	}
}

func TestWaitingThenProtectedSkipsInCycleStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{General: config.GeneralConfig{PrivateTrackerHandling: "keep", PublicTrackerHandling: "remove"}}
	m := jobs.NewManager(cfg, logger, "")
	defer m.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("unexpected mutation")
		}
		fmt.Fprint(w, `{"totalRecords":1,"records":[{"id":1,"downloadId":"abc","trackedDownloadStatus":"warning","trackedDownloadState":"downloading"}]}`)
	}))
	defer server.Close()
	m.RegisterArrClient("radarr", arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL}))
	m.RegisterDownloadClient("qbit", &safetyClient{private: true, torrents: []downloadclient.Torrent{{Hash: "abc"}}})
	m.RegisterJob(NewFailedDownloadsJob("failed", &config.JobConfig{Enabled: true}, &config.JobDefaultsConfig{MaxStrikes: 2}, m, logger, false))
	for _, reason := range []string{"waiting_for_strikes", "private_tracker_policy", "private_tracker_policy"} {
		if err := m.RunAll(context.Background()); err != nil {
			t.Fatal(err)
		}
		stats := m.GetLastStats()
		if stats.ItemsFound["failed"] != 1 || stats.ItemsRemoved["failed"] != 0 {
			t.Fatalf("stats=%+v", stats)
		}
		if len(stats.SkipReasons["failed"]) != 1 || stats.SkipReasons["failed"][reason] != 1 {
			t.Fatalf("reasons=%v", stats.SkipReasons)
		}
	}
}
