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

func TestQBitNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  QBittorrentConfig
		wantErr bool
	}{
		{
			name: "valid config with all fields",
			config: QBittorrentConfig{
				BaseURL:  "http://localhost:8080",
				Username: "admin",
				Password: "adminpass",
				Timeout:  30 * time.Second,
				SkipTLS:  false,
			},
			wantErr: false,
		},
		{
			name: "valid config with default timeout",
			config: QBittorrentConfig{
				BaseURL:  "http://localhost:8080",
				Username: "admin",
				Password: "adminpass",
			},
			wantErr: false,
		},
		{
			name: "trailing slash in baseURL is trimmed",
			config: QBittorrentConfig{
				BaseURL:  "http://localhost:8080/",
				Username: "admin",
				Password: "adminpass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewQBittorrentClient(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				assert.Equal(t, "qBittorrent", client.Name())
				assert.NotNil(t, client.http)
				assert.NotNil(t, client.logger)
			}
		})
	}
}

func TestQBitLogin(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful login",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v2/auth/login", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

				// Set SID cookie
				http.SetCookie(w, &http.Cookie{
					Name:  "SID",
					Value: "test_session_id",
				})
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Ok."))
			},
			wantErr: false,
		},
		{
			name: "login failed - wrong credentials",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Fails."))
			},
			wantErr:     true,
			errContains: "login failed",
		},
		{
			name: "login failed - http error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("Unauthorized"))
			},
			wantErr:     true,
			errContains: "login failed with status 401",
		},
		{
			name: "login succeeded but no SID cookie",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Ok."))
			},
			wantErr:     true,
			errContains: "SID cookie not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := QBittorrentConfig{
				BaseURL:  server.URL,
				Username: "admin",
				Password: "adminpass",
			}

			client, err := NewQBittorrentClient(cfg)
			require.NoError(t, err)

			err = client.Login(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, client.sid)
			}
		})
	}
}

func TestQBitGetTorrents(t *testing.T) {
	mockTorrents := []qBitTorrentInfo{
		{
			Hash:         "abc123",
			Name:         "Test Torrent 1",
			State:        "downloading",
			Progress:     0.5,
			Size:         1000000000,
			Downloaded:   500000000,
			Uploaded:     100000000,
			Dlspeed:      1000000,
			Upspeed:      500000,
			Ratio:        0.2,
			SeedingTime:  3600,
			AddedOn:      1640000000,
			CompletionOn: 0,
			SavePath:     "/downloads",
			Category:     "movies",
			Tags:         "hd,1080p",
			TrackerHost:  "tracker.example.com",
		},
		{
			Hash:         "def456",
			Name:         "Test Torrent 2",
			State:        "uploading",
			Progress:     1.0,
			Size:         2000000000,
			Downloaded:   2000000000,
			Uploaded:     2000000000,
			Dlspeed:      0,
			Upspeed:      2000000,
			Ratio:        1.0,
			SeedingTime:  7200,
			AddedOn:      1640000000,
			CompletionOn: 1640003600,
			SavePath:     "/downloads",
			Category:     "tv",
			Tags:         "",
		},
	}

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectLogin    bool
		wantCount      int
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful retrieval with existing session",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test_sid"})
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("Ok."))
					return
				}

				assert.Equal(t, "/api/v2/torrents/info", r.URL.Path)
				assert.Equal(t, "SID=test_sid", r.Header.Get("Cookie"))
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(mockTorrents)
			},
			expectLogin: true,
			wantCount:   2,
			wantErr:     false,
		},
		{
			name: "session expired - auto re-login",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "new_sid"})
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("Ok."))
					return
				}

				if r.URL.Path == "/api/v2/torrents/info" {
					cookie := r.Header.Get("Cookie")
					if cookie == "SID=test_sid" {
						// First call with old session - return forbidden
						w.WriteHeader(http.StatusForbidden)
						return
					}
					// Second call with new session - return data
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(mockTorrents)
				}
			},
			expectLogin: true,
			wantCount:   2,
			wantErr:     false,
		},
		{
			name: "empty torrent list",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test_sid"})
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("Ok."))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]qBitTorrentInfo{})
			},
			expectLogin: true,
			wantCount:   0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := QBittorrentConfig{
				BaseURL:  server.URL,
				Username: "admin",
				Password: "adminpass",
			}

			client, err := NewQBittorrentClient(cfg)
			require.NoError(t, err)

			if tt.expectLogin && tt.name != "session expired - auto re-login" {
				client.sid = "test_sid"
			}

			torrents, err := client.GetTorrents(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, torrents, tt.wantCount)

				if tt.wantCount > 0 {
					// Verify first torrent conversion
					assert.Equal(t, "abc123", torrents[0].Hash)
					assert.Equal(t, "Test Torrent 1", torrents[0].Name)
					assert.Equal(t, StateDownloading, torrents[0].State)
					assert.Equal(t, 0.5, torrents[0].Progress)
					assert.Len(t, torrents[0].Tags, 2)
					assert.Contains(t, torrents[0].Tags, "hd")
					assert.Contains(t, torrents[0].Tags, "1080p")
				}
			}
		})
	}
}

func TestQBitGetTorrent(t *testing.T) {
	mockTorrent := qBitTorrentInfo{
		Hash:       "abc123",
		Name:       "Test Torrent",
		State:      "downloading",
		Progress:   0.75,
		Size:       1000000000,
		Downloaded: 750000000,
		Category:   "movies",
	}

	tests := []struct {
		name           string
		hash           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful single torrent retrieval",
			hash: "abc123",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test_sid"})
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("Ok."))
					return
				}

				assert.Contains(t, r.URL.String(), "hashes=abc123")
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]qBitTorrentInfo{mockTorrent})
			},
			wantErr: false,
		},
		{
			name: "torrent not found",
			hash: "nonexistent",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test_sid"})
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("Ok."))
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]qBitTorrentInfo{})
			},
			wantErr:     true,
			errContains: "torrent not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := QBittorrentConfig{
				BaseURL:  server.URL,
				Username: "admin",
				Password: "adminpass",
			}

			client, err := NewQBittorrentClient(cfg)
			require.NoError(t, err)

			torrent, err := client.GetTorrent(context.Background(), tt.hash)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, torrent)
				assert.Equal(t, tt.hash, torrent.Hash)
				assert.Equal(t, "Test Torrent", torrent.Name)
			}
		})
	}
}

func TestQBitDeleteTorrent(t *testing.T) {
	tests := []struct {
		name           string
		hash           string
		deleteFiles    bool
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
	}{
		{
			name:        "delete with files",
			hash:        "abc123",
			deleteFiles: true,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test_sid"})
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("Ok."))
					return
				}

				assert.Equal(t, "/api/v2/torrents/delete", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)

				_ = r.ParseForm()
				assert.Equal(t, "abc123", r.FormValue("hashes"))
				assert.Equal(t, "true", r.FormValue("deleteFiles"))

				w.WriteHeader(http.StatusOK)
			},
			wantErr: false,
		},
		{
			name:        "delete without files",
			hash:        "def456",
			deleteFiles: false,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test_sid"})
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("Ok."))
					return
				}

				_ = r.ParseForm()
				assert.Equal(t, "def456", r.FormValue("hashes"))
				assert.Equal(t, "false", r.FormValue("deleteFiles"))

				w.WriteHeader(http.StatusOK)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := QBittorrentConfig{
				BaseURL:  server.URL,
				Username: "admin",
				Password: "adminpass",
			}

			client, err := NewQBittorrentClient(cfg)
			require.NoError(t, err)

			err = client.DeleteTorrent(context.Background(), tt.hash, tt.deleteFiles)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQBitPauseTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test_sid"})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}

		assert.Equal(t, "/api/v2/torrents/pause", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		_ = r.ParseForm()
		assert.Equal(t, "abc123", r.FormValue("hashes"))

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := QBittorrentConfig{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "adminpass",
	}

	client, err := NewQBittorrentClient(cfg)
	require.NoError(t, err)

	err = client.PauseTorrent(context.Background(), "abc123")
	assert.NoError(t, err)
}

func TestQBitResumeTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test_sid"})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}

		assert.Equal(t, "/api/v2/torrents/resume", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		_ = r.ParseForm()
		assert.Equal(t, "abc123", r.FormValue("hashes"))

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := QBittorrentConfig{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "adminpass",
	}

	client, err := NewQBittorrentClient(cfg)
	require.NoError(t, err)

	err = client.ResumeTorrent(context.Background(), "abc123")
	assert.NoError(t, err)
}

func TestQBitSessionExpired(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "new_sid"})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}

		if r.URL.Path == "/api/v2/torrents/info" {
			callCount++
			if callCount == 1 {
				// First call - session expired
				w.WriteHeader(http.StatusForbidden)
				return
			}
			// Second call after re-login - success
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]qBitTorrentInfo{})
		}
	}))
	defer server.Close()

	cfg := QBittorrentConfig{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "adminpass",
	}

	client, err := NewQBittorrentClient(cfg)
	require.NoError(t, err)

	// Set an old session ID
	client.sid = "old_sid"

	// This should trigger re-login
	torrents, err := client.GetTorrents(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, torrents)
	assert.Equal(t, 2, callCount) // Should have called twice
}

func TestMapQBitState(t *testing.T) {
	tests := []struct {
		qbitState     string
		expectedState TorrentState
	}{
		{"downloading", StateDownloading},
		{"metaDL", StateDownloading},
		{"forcedDL", StateDownloading},
		{"allocating", StateDownloading},
		{"uploading", StateSeeding},
		{"stalledUP", StateSeeding},
		{"forcedUP", StateSeeding},
		{"pausedDL", StatePaused},
		{"pausedUP", StatePaused},
		{"stalledDL", StateStalled},
		{"error", StateError},
		{"missingFiles", StateError},
		{"queuedDL", StateQueued},
		{"queuedUP", StateQueued},
		{"checkingDL", StateQueued},
		{"checkingUP", StateQueued},
		{"checkingResumeData", StateQueued},
		{"unknown", StateQueued}, // Default case
	}

	for _, tt := range tests {
		t.Run(tt.qbitState, func(t *testing.T) {
			result := mapQBitState(tt.qbitState)
			assert.Equal(t, tt.expectedState, result)
		})
	}
}
