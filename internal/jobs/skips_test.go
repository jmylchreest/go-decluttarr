package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/downloadclient"
)

type skipPolicyClient struct {
	downloadclient.Client
	private bool
	err     error
}

func (c *skipPolicyClient) IsPrivateTracker(context.Context, string) (bool, error) {
	return c.private, c.err
}

type skipTestJob struct {
	name string
	run  func(context.Context) error
}

func (j *skipTestJob) Name() string                  { return j.name }
func (j *skipTestJob) Enabled() bool                 { return true }
func (j *skipTestJob) Run(ctx context.Context) error { return j.run(ctx) }

// Deliberately no Stats method: a skip-only job must still appear in the summary.

func TestPolicySkipReasonsAtInfoAndResetEachCycle(t *testing.T) {
	for _, tc := range []struct {
		name, reason, public, private                            string
		privateTorrent, protected, shelfarr, missing, trackerErr bool
	}{
		{name: "missing download", reason: "download_unavailable", missing: true},
		{name: "protected tag", reason: "protected_tag", protected: true},
		{name: "Shelfarr", reason: "shelfarr_category", shelfarr: true},
		{name: "tracker unavailable", reason: "tracker_status_unavailable", trackerErr: true},
		{name: "private keep", reason: "private_tracker_policy", privateTorrent: true, private: "keep"},
		{name: "private skip", reason: "private_tracker_policy", privateTorrent: true, private: "skip"},
		{name: "public keep", reason: "public_tracker_policy", public: "keep"},
		{name: "unknown policy", reason: "unknown_tracker_policy", public: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			cfg := &config.Config{General: config.GeneralConfig{PublicTrackerHandling: tc.public, PrivateTrackerHandling: tc.private, ProtectedTag: "Keep"}}
			if tc.shelfarr {
				cfg.Instances.Shelfarr = []config.ShelfarrConfig{{ProtectedCategories: []string{"books"}}}
			}
			m := NewManager(cfg, slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo})), "")
			defer m.Close()
			torrent := &downloadclient.Torrent{Hash: "secret-hash", Category: "books", Name: "private-title"}
			if tc.protected {
				torrent.Tags = []string{"Keep"}
			}
			dc := &skipPolicyClient{private: tc.privateTorrent}
			if tc.trackerErr {
				dc.err = errors.New("lookup unavailable")
			}
			active := true
			for _, name := range []string{"first", "second"} {
				m.RegisterJob(&skipTestJob{name: name, run: func(ctx context.Context) error {
					if !active {
						return nil
					}
					action := ""
					if tc.missing {
						action = m.GetRemovalAction(ctx, "secret-hash")
					} else {
						action = m.GetTorrentRemovalAction(ctx, torrent, dc)
					}
					if action != "skip" {
						t.Errorf("action=%s", action)
					}
					return nil
				}})
			}
			for cycle := 0; cycle < 2; cycle++ {
				output.Reset()
				if err := m.RunAll(context.Background()); err != nil {
					t.Fatal(err)
				}
				want := 1
				if cycle == 1 {
					want = 0
				}
				for _, name := range []string{"first", "second"} {
					if got := m.GetLastStats().SkipReasons[name][tc.reason]; got != want {
						t.Fatalf("cycle=%d job=%s skips=%d", cycle, name, got)
					}
				}
				decoder := json.NewDecoder(&output)
				summaries := 0
				for {
					var record struct {
						Message string `json:"msg"`
						Level   string `json:"level"`
						Totals  struct {
							Skipped int `json:"skipped"`
						} `json:"totals"`
						Jobs map[string]JobResult `json:"jobs"`
					}
					err := decoder.Decode(&record)
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					if record.Message != "cycle complete" {
						continue
					}
					summaries++
					if record.Level != "INFO" || record.Totals.Skipped != 2*want {
						t.Fatalf("summary=%+v", record)
					}
					if want == 0 && len(record.Jobs) != 0 {
						t.Fatal("stale skip-only jobs")
					}
					if want > 0 && record.Jobs["first"].Skipped[tc.reason] != 1 {
						t.Fatal("missing INFO reason")
					}
				}
				if summaries != 1 {
					t.Fatalf("summaries=%d", summaries)
				}
				active = false
			}
		})
	}
}

func TestSkipRecorderPreservesCountsOnJobFailure(t *testing.T) {
	m := NewManager(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), "")
	defer m.Close()
	m.RegisterJob(&skipTestJob{name: "failed", run: func(ctx context.Context) error {
		RecordSkip(ctx, "waiting_for_strikes")
		return errors.New("queue failed later")
	}})
	if m.RunAll(context.Background()) == nil {
		t.Fatal("job error lost")
	}
	if m.GetLastStats().SkipReasons["failed"]["waiting_for_strikes"] != 1 {
		t.Fatal("partial job skips lost")
	}
	// Public policy helpers also remain usable outside a managed cycle.
	RecordSkip(context.Background(), "protected_tag")
}
