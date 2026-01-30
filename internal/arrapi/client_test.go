package arrapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name       string
		cfg        ClientConfig
		wantName   string
		wantAPIVer string
		wantURL    string
	}{
		{
			name: "default config",
			cfg: ClientConfig{
				Name:    "test-arr",
				BaseURL: "http://localhost:8989",
				APIKey:  "testkey123",
			},
			wantName:   "test-arr",
			wantAPIVer: "v3",
			wantURL:    "http://localhost:8989",
		},
		{
			name: "custom api version",
			cfg: ClientConfig{
				Name:       "lidarr",
				BaseURL:    "http://localhost:8686",
				APIKey:     "key456",
				APIVersion: "v1",
			},
			wantName:   "lidarr",
			wantAPIVer: "v1",
			wantURL:    "http://localhost:8686",
		},
		{
			name: "trailing slash removal",
			cfg: ClientConfig{
				Name:    "radarr",
				BaseURL: "http://localhost:7878/",
				APIKey:  "key789",
			},
			wantName:   "radarr",
			wantAPIVer: "v3",
			wantURL:    "http://localhost:7878",
		},
		{
			name: "with custom timeout",
			cfg: ClientConfig{
				Name:    "sonarr",
				BaseURL: "http://localhost:8989",
				APIKey:  "keyabc",
				Timeout: 60 * time.Second,
			},
			wantName:   "sonarr",
			wantAPIVer: "v3",
			wantURL:    "http://localhost:8989",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.cfg)
			if client == nil {
				t.Fatal("expected non-nil client")
			}

			if client.name != tt.wantName {
				t.Errorf("name = %q, want %q", client.name, tt.wantName)
			}

			if client.apiVersion != tt.wantAPIVer {
				t.Errorf("apiVersion = %q, want %q", client.apiVersion, tt.wantAPIVer)
			}

			if client.baseURL != tt.wantURL {
				t.Errorf("baseURL = %q, want %q", client.baseURL, tt.wantURL)
			}

			if client.apiKey != tt.cfg.APIKey {
				t.Errorf("apiKey = %q, want %q", client.apiKey, tt.cfg.APIKey)
			}
		})
	}
}

func TestGetQueue(t *testing.T) {
	mockQueue := QueueResponse{
		Page:         1,
		PageSize:     1000,
		TotalRecords: 2,
		Records: []QueueItem{
			{
				ID:                    1,
				Title:                 "Test Episode",
				Status:                "downloading",
				TrackedDownloadStatus: "ok",
				TrackedDownloadState:  "downloading",
				DownloadID:            "download123",
				Protocol:              "torrent",
				DownloadClient:        "transmission",
				Size:                  1073741824,
				Sizeleft:              536870912,
				Added:                 time.Now(),
				SeriesID:              intPtr(10),
				EpisodeID:             intPtr(100),
				SeasonNumber:          intPtr(1),
			},
			{
				ID:                    2,
				Title:                 "Test Movie",
				Status:                "completed",
				TrackedDownloadStatus: "ok",
				TrackedDownloadState:  "importPending",
				DownloadID:            "download456",
				Protocol:              "usenet",
				DownloadClient:        "sabnzbd",
				Size:                  2147483648,
				Sizeleft:              0,
				Added:                 time.Now(),
				MovieID:               intPtr(20),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if !containsPath(r.URL.Path, "/api/v3/queue") {
			t.Errorf("expected path to contain /api/v3/queue, got %s", r.URL.Path)
		}

		if r.Header.Get("X-Api-Key") != "testkey" {
			t.Errorf("expected X-Api-Key header, got %s", r.Header.Get("X-Api-Key"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockQueue)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		Name:    "test",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	queue, err := client.GetQueue(context.Background())
	if err != nil {
		t.Fatalf("GetQueue failed: %v", err)
	}

	if len(queue) != 2 {
		t.Errorf("expected 2 queue items, got %d", len(queue))
	}

	if queue[0].Title != "Test Episode" {
		t.Errorf("expected title 'Test Episode', got %q", queue[0].Title)
	}

	if queue[1].Title != "Test Movie" {
		t.Errorf("expected title 'Test Movie', got %q", queue[1].Title)
	}
}

func TestDeleteQueueItem(t *testing.T) {
	tests := []struct {
		name             string
		itemID           int
		opts             DeleteOptions
		wantQueryParams  map[string]string
	}{
		{
			name:   "delete with no options",
			itemID: 123,
			opts:   DeleteOptions{},
			wantQueryParams: map[string]string{},
		},
		{
			name:   "delete with remove from client",
			itemID: 456,
			opts: DeleteOptions{
				RemoveFromClient: true,
			},
			wantQueryParams: map[string]string{
				"removeFromClient": "true",
			},
		},
		{
			name:   "delete with blocklist",
			itemID: 789,
			opts: DeleteOptions{
				Blocklist: true,
			},
			wantQueryParams: map[string]string{
				"blocklist": "true",
			},
		},
		{
			name:   "delete with skip redownload",
			itemID: 101,
			opts: DeleteOptions{
				SkipRedownload: true,
			},
			wantQueryParams: map[string]string{
				"skipRedownload": "true",
			},
		},
		{
			name:   "delete with all options",
			itemID: 202,
			opts: DeleteOptions{
				RemoveFromClient: true,
				Blocklist:        true,
				SkipRedownload:   true,
			},
			wantQueryParams: map[string]string{
				"removeFromClient": "true",
				"blocklist":        "true",
				"skipRedownload":   "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Errorf("expected DELETE, got %s", r.Method)
				}

				if !containsPath(r.URL.Path, "/api/v3/queue/") {
					t.Errorf("expected path to contain /api/v3/queue/, got %s", r.URL.Path)
				}

				query := r.URL.Query()
				for key, wantVal := range tt.wantQueryParams {
					if got := query.Get(key); got != wantVal {
						t.Errorf("query param %s = %q, want %q", key, got, wantVal)
					}
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := NewClient(ClientConfig{
				Name:    "test",
				BaseURL: server.URL,
				APIKey:  "testkey",
			})

			err := client.DeleteQueueItem(context.Background(), tt.itemID, tt.opts)
			if err != nil {
				t.Fatalf("DeleteQueueItem failed: %v", err)
			}
		})
	}
}

func TestGetSystemStatus(t *testing.T) {
	mockStatus := SystemStatus{
		AppName:      "Sonarr",
		InstanceName: "Sonarr-Test",
		Version:      "3.0.10.1567",
		BuildTime:    "2023-01-15T10:00:00Z",
		IsDebug:      false,
		IsProduction: true,
		IsLinux:      true,
		IsDocker:     true,
		Branch:       "main",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if !containsPath(r.URL.Path, "/api/v3/system/status") {
			t.Errorf("expected path to contain /api/v3/system/status, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockStatus)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		Name:    "test",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	status, err := client.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSystemStatus failed: %v", err)
	}

	if status.AppName != mockStatus.AppName {
		t.Errorf("AppName = %q, want %q", status.AppName, mockStatus.AppName)
	}

	if status.Version != mockStatus.Version {
		t.Errorf("Version = %q, want %q", status.Version, mockStatus.Version)
	}

	if status.IsLinux != mockStatus.IsLinux {
		t.Errorf("IsLinux = %v, want %v", status.IsLinux, mockStatus.IsLinux)
	}
}

func TestGetMonitoredStatus(t *testing.T) {
	tests := []struct {
		name         string
		entityType   string
		entityID     int
		monitored    bool
	}{
		{
			name:       "monitored series",
			entityType: "series",
			entityID:   123,
			monitored:  true,
		},
		{
			name:       "unmonitored movie",
			entityType: "movie",
			entityID:   456,
			monitored:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"monitored": tt.monitored,
				})
			}))
			defer server.Close()

			client := NewClient(ClientConfig{
				Name:    "test",
				BaseURL: server.URL,
				APIKey:  "testkey",
			})

			monitored, err := client.GetMonitoredStatus(context.Background(), tt.entityType, tt.entityID)
			if err != nil {
				t.Fatalf("GetMonitoredStatus failed: %v", err)
			}

			if monitored != tt.monitored {
				t.Errorf("monitored = %v, want %v", monitored, tt.monitored)
			}
		})
	}
}

func TestAPIError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		wantErrContains string
	}{
		{
			name:            "404 not found",
			statusCode:      http.StatusNotFound,
			responseBody:    `{"error": "Not found"}`,
			wantErrContains: "404",
		},
		{
			name:            "401 unauthorized",
			statusCode:      http.StatusUnauthorized,
			responseBody:    `{"error": "Invalid API key"}`,
			wantErrContains: "401",
		},
		{
			name:            "500 internal server error",
			statusCode:      http.StatusInternalServerError,
			responseBody:    `{"error": "Internal server error"}`,
			wantErrContains: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient(ClientConfig{
				Name:    "test",
				BaseURL: server.URL,
				APIKey:  "testkey",
			})

			_, err := client.GetQueue(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !containsString(err.Error(), tt.wantErrContains) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		Name:    "test",
		BaseURL: server.URL,
		APIKey:  "testkey",
		Timeout: 50 * time.Millisecond,
	})

	ctx := context.Background()
	_, err := client.GetQueue(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		Name:    "test",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.GetQueue(ctx)
	if err == nil {
		t.Fatal("expected context timeout error, got nil")
	}
}

// Helper functions

func intPtr(i int) *int {
	return &i
}

func containsPath(path, substr string) bool {
	return len(path) >= len(substr) && path[:len(substr)] == substr ||
	       len(path) > len(substr) && containsString(path, substr)
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
