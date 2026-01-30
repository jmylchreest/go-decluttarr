package downloadclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSABNewClient(t *testing.T) {
	tests := []struct {
		name   string
		config SABnzbdConfig
	}{
		{
			name: "valid config with all fields",
			config: SABnzbdConfig{
				BaseURL: "http://localhost:8080",
				APIKey:  "test_api_key",
				Timeout: 30 * time.Second,
			},
		},
		{
			name: "valid config with default timeout",
			config: SABnzbdConfig{
				BaseURL: "http://localhost:8080",
				APIKey:  "test_api_key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewSABnzbdClient(tt.config)
			require.NotNil(t, client)
			assert.Equal(t, "SABnzbd", client.Name())
			assert.NotNil(t, client.http)
			assert.NotNil(t, client.logger)
			assert.Equal(t, tt.config.BaseURL, client.baseURL)
			assert.Equal(t, tt.config.APIKey, client.apiKey)
		})
	}
}

func TestSABGetQueue(t *testing.T) {
	mockQueue := SABnzbdQueueResponse{
		Queue: struct {
			Slots []SABnzbdSlot `json:"slots"`
		}{
			Slots: []SABnzbdSlot{
				{
					NzoID:      "SABnzbd_nzo_abc123",
					Filename:   "Test.Download.mkv",
					Status:     "Downloading",
					Size:       "1.5 GB",
					Sizeleft:   "500 MB",
					Percentage: "66.7",
					Category:   "movies",
					MBLeft:     500,
					MB:         1536,
				},
				{
					NzoID:      "SABnzbd_nzo_def456",
					Filename:   "Another.Download.mkv",
					Status:     "Paused",
					Size:       "2.0 GB",
					Sizeleft:   "2.0 GB",
					Percentage: "0",
					Category:   "tv",
					MBLeft:     2048,
					MB:         2048,
				},
			},
		},
	}

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantCount      int
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful queue retrieval",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api", r.URL.Path)
				assert.Equal(t, "queue", r.URL.Query().Get("mode"))
				assert.Equal(t, "test_api_key", r.URL.Query().Get("apikey"))
				assert.Equal(t, "json", r.URL.Query().Get("output"))

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(mockQueue)
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "empty queue",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				emptyQueue := SABnzbdQueueResponse{
					Queue: struct {
						Slots []SABnzbdSlot `json:"slots"`
					}{
						Slots: []SABnzbdSlot{},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(emptyQueue)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("Internal Server Error"))
			},
			wantErr:     true,
			errContains: "failed to decode queue response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := SABnzbdConfig{
				BaseURL: server.URL,
				APIKey:  "test_api_key",
			}

			client := NewSABnzbdClient(cfg)
			slots, err := client.GetQueue(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, slots, tt.wantCount)

				if tt.wantCount > 0 {
					assert.Equal(t, "SABnzbd_nzo_abc123", slots[0].NzoID)
					assert.Equal(t, "Test.Download.mkv", slots[0].Filename)
					assert.Equal(t, "Downloading", slots[0].Status)
					assert.Equal(t, "movies", slots[0].Category)
				}
			}
		})
	}
}

func TestSABGetHistory(t *testing.T) {
	mockHistory := SABnzbdHistoryResponse{
		History: struct {
			Slots []SABnzbdHistorySlot `json:"slots"`
		}{
			Slots: []SABnzbdHistorySlot{
				{
					NzoID:    "SABnzbd_nzo_hist1",
					Name:     "Completed.Download.mkv",
					Status:   "Completed",
					Bytes:    "1610612736",
					Category: "movies",
					Storage:  "/downloads/movies/Completed.Download.mkv",
				},
				{
					NzoID:    "SABnzbd_nzo_hist2",
					Name:     "Failed.Download.mkv",
					Status:   "Failed",
					Bytes:    "0",
					Category: "tv",
					Storage:  "",
				},
			},
		},
	}

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantCount      int
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful history retrieval",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api", r.URL.Path)
				assert.Equal(t, "history", r.URL.Query().Get("mode"))
				assert.Equal(t, "test_api_key", r.URL.Query().Get("apikey"))
				assert.Equal(t, "json", r.URL.Query().Get("output"))

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(mockHistory)
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "empty history",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				emptyHistory := SABnzbdHistoryResponse{
					History: struct {
						Slots []SABnzbdHistorySlot `json:"slots"`
					}{
						Slots: []SABnzbdHistorySlot{},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(emptyHistory)
			},
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := SABnzbdConfig{
				BaseURL: server.URL,
				APIKey:  "test_api_key",
			}

			client := NewSABnzbdClient(cfg)
			slots, err := client.GetHistory(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, slots, tt.wantCount)

				if tt.wantCount > 0 {
					assert.Equal(t, "SABnzbd_nzo_hist1", slots[0].NzoID)
					assert.Equal(t, "Completed.Download.mkv", slots[0].Name)
					assert.Equal(t, "Completed", slots[0].Status)
				}
			}
		})
	}
}

func TestSABDeleteSlot(t *testing.T) {
	tests := []struct {
		name           string
		nzoID          string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
	}{
		{
			name:  "successful delete",
			nzoID: "SABnzbd_nzo_abc123",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api", r.URL.Path)
				assert.Equal(t, "queue", r.URL.Query().Get("mode"))
				assert.Equal(t, "delete", r.URL.Query().Get("name"))
				assert.Equal(t, "SABnzbd_nzo_abc123", r.URL.Query().Get("value"))
				assert.Equal(t, "test_api_key", r.URL.Query().Get("apikey"))

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status": true}`))
			},
			wantErr: false,
		},
		{
			name:  "server error on delete",
			nzoID: "SABnzbd_nzo_error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: false, // SABnzbd client doesn't check HTTP status on delete
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := SABnzbdConfig{
				BaseURL: server.URL,
				APIKey:  "test_api_key",
			}

			client := NewSABnzbdClient(cfg)
			err := client.DeleteSlot(context.Background(), tt.nzoID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSABPauseSlot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api", r.URL.Path)
		assert.Equal(t, "queue", r.URL.Query().Get("mode"))
		assert.Equal(t, "pause", r.URL.Query().Get("name"))
		assert.Equal(t, "SABnzbd_nzo_abc123", r.URL.Query().Get("value"))
		assert.Equal(t, "test_api_key", r.URL.Query().Get("apikey"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": true}`))
	}))
	defer server.Close()

	cfg := SABnzbdConfig{
		BaseURL: server.URL,
		APIKey:  "test_api_key",
	}

	client := NewSABnzbdClient(cfg)
	err := client.PauseSlot(context.Background(), "SABnzbd_nzo_abc123")
	assert.NoError(t, err)
}

func TestSABResumeSlot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api", r.URL.Path)
		assert.Equal(t, "queue", r.URL.Query().Get("mode"))
		assert.Equal(t, "resume", r.URL.Query().Get("name"))
		assert.Equal(t, "SABnzbd_nzo_abc123", r.URL.Query().Get("value"))
		assert.Equal(t, "test_api_key", r.URL.Query().Get("apikey"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": true}`))
	}))
	defer server.Close()

	cfg := SABnzbdConfig{
		BaseURL: server.URL,
		APIKey:  "test_api_key",
	}

	client := NewSABnzbdClient(cfg)
	err := client.ResumeSlot(context.Background(), "SABnzbd_nzo_abc123")
	assert.NoError(t, err)
}

func TestSABGetTorrents(t *testing.T) {
	mockQueue := SABnzbdQueueResponse{
		Queue: struct {
			Slots []SABnzbdSlot `json:"slots"`
		}{
			Slots: []SABnzbdSlot{
				{
					NzoID:      "SABnzbd_nzo_abc123",
					Filename:   "Test.Download.mkv",
					Status:     "Downloading",
					Percentage: "75",
					Category:   "movies",
					MBLeft:     256,
					MB:         1024,
				},
				{
					NzoID:      "SABnzbd_nzo_def456",
					Filename:   "Paused.Download.mkv",
					Status:     "Paused",
					Percentage: "0",
					Category:   "tv",
					MBLeft:     2048,
					MB:         2048,
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockQueue)
	}))
	defer server.Close()

	cfg := SABnzbdConfig{
		BaseURL: server.URL,
		APIKey:  "test_api_key",
	}

	client := NewSABnzbdClient(cfg)
	torrents, err := client.GetTorrents(context.Background())

	require.NoError(t, err)
	require.Len(t, torrents, 2)

	// Verify first torrent conversion
	assert.Equal(t, "SABnzbd_nzo_abc123", torrents[0].Hash)
	assert.Equal(t, "Test.Download.mkv", torrents[0].Name)
	assert.Equal(t, StateDownloading, torrents[0].State)
	assert.Equal(t, 0.75, torrents[0].Progress)
	assert.Equal(t, "movies", torrents[0].Category)
	assert.Equal(t, int64(1024*1024*1024), torrents[0].Size)
	assert.Equal(t, int64(768*1024*1024), torrents[0].Downloaded) // 75% of 1024MB

	// Verify second torrent (paused)
	assert.Equal(t, StatePaused, torrents[1].State)
	assert.Equal(t, 0.0, torrents[1].Progress)
}

func TestSABGetTorrent(t *testing.T) {
	mockQueue := SABnzbdQueueResponse{
		Queue: struct {
			Slots []SABnzbdSlot `json:"slots"`
		}{
			Slots: []SABnzbdSlot{
				{
					NzoID:      "SABnzbd_nzo_abc123",
					Filename:   "Test.Download.mkv",
					Status:     "Downloading",
					Percentage: "50",
					Category:   "movies",
					MBLeft:     512,
					MB:         1024,
				},
				{
					NzoID:    "SABnzbd_nzo_def456",
					Filename: "Other.Download.mkv",
					Status:   "Queued",
				},
			},
		},
	}

	tests := []struct {
		name        string
		nzoID       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "found torrent",
			nzoID:   "SABnzbd_nzo_abc123",
			wantErr: false,
		},
		{
			name:        "not found",
			nzoID:       "SABnzbd_nzo_notfound",
			wantErr:     true,
			errContains: "slot not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(mockQueue)
			}))
			defer server.Close()

			cfg := SABnzbdConfig{
				BaseURL: server.URL,
				APIKey:  "test_api_key",
			}

			client := NewSABnzbdClient(cfg)
			torrent, err := client.GetTorrent(context.Background(), tt.nzoID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, torrent)
				assert.Equal(t, tt.nzoID, torrent.Hash)
			}
		})
	}
}

func TestSABDeleteTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "delete", r.URL.Query().Get("name"))
		assert.Equal(t, "SABnzbd_nzo_abc123", r.URL.Query().Get("value"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := SABnzbdConfig{
		BaseURL: server.URL,
		APIKey:  "test_api_key",
	}

	client := NewSABnzbdClient(cfg)
	err := client.DeleteTorrent(context.Background(), "SABnzbd_nzo_abc123", true)
	assert.NoError(t, err)
}

func TestSABPauseTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "pause", r.URL.Query().Get("name"))
		assert.Equal(t, "SABnzbd_nzo_abc123", r.URL.Query().Get("value"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := SABnzbdConfig{
		BaseURL: server.URL,
		APIKey:  "test_api_key",
	}

	client := NewSABnzbdClient(cfg)
	err := client.PauseTorrent(context.Background(), "SABnzbd_nzo_abc123")
	assert.NoError(t, err)
}

func TestSABResumeTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "resume", r.URL.Query().Get("name"))
		assert.Equal(t, "SABnzbd_nzo_abc123", r.URL.Query().Get("value"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := SABnzbdConfig{
		BaseURL: server.URL,
		APIKey:  "test_api_key",
	}

	client := NewSABnzbdClient(cfg)
	err := client.ResumeTorrent(context.Background(), "SABnzbd_nzo_abc123")
	assert.NoError(t, err)
}

func TestSABSlotToTorrent(t *testing.T) {
	tests := []struct {
		name          string
		slot          SABnzbdSlot
		expectedState TorrentState
		expectedProg  float64
	}{
		{
			name: "downloading slot",
			slot: SABnzbdSlot{
				NzoID:      "nzo_123",
				Filename:   "test.mkv",
				Status:     "Downloading",
				Percentage: "75.5",
				Category:   "movies",
				MB:         1024,
				MBLeft:     256,
			},
			expectedState: StateDownloading,
			expectedProg:  0.755,
		},
		{
			name: "paused slot",
			slot: SABnzbdSlot{
				NzoID:      "nzo_456",
				Filename:   "paused.mkv",
				Status:     "Paused",
				Percentage: "10",
				Category:   "tv",
				MB:         2048,
				MBLeft:     1843.2,
			},
			expectedState: StatePaused,
			expectedProg:  0.1,
		},
		{
			name: "queued slot",
			slot: SABnzbdSlot{
				NzoID:      "nzo_789",
				Filename:   "queued.mkv",
				Status:     "Queued",
				Percentage: "0",
				Category:   "movies",
				MB:         512,
				MBLeft:     512,
			},
			expectedState: StateQueued,
			expectedProg:  0.0,
		},
		{
			name: "fetching slot",
			slot: SABnzbdSlot{
				NzoID:      "nzo_fetch",
				Filename:   "fetching.mkv",
				Status:     "Fetching",
				Percentage: "5",
				Category:   "tv",
				MB:         1000,
				MBLeft:     950,
			},
			expectedState: StateDownloading,
			expectedProg:  0.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SABnzbdConfig{
				BaseURL: "http://localhost",
				APIKey:  "test",
			}
			client := NewSABnzbdClient(cfg)

			torrent, err := client.slotToTorrent(tt.slot)
			require.NoError(t, err)

			assert.Equal(t, tt.slot.NzoID, torrent.Hash)
			assert.Equal(t, tt.slot.Filename, torrent.Name)
			assert.Equal(t, tt.expectedState, torrent.State)
			assert.InDelta(t, tt.expectedProg, torrent.Progress, 0.001)
			assert.Equal(t, tt.slot.Category, torrent.Category)
			assert.Equal(t, int64(tt.slot.MB*1024*1024), torrent.Size)
		})
	}
}
