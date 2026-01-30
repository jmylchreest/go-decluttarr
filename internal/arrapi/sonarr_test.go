package arrapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSonarrGetSeries(t *testing.T) {
	mockSeries := Series{
		ID:          123,
		Title:       "Test Series",
		SeasonCount: 5,
		Status:      "continuing",
		Overview:    "A test series",
		Monitored:   true,
		Path:        "/tv/test-series",
		Added:       time.Now(),
		Statistics: Statistics{
			EpisodeFileCount:  50,
			EpisodeCount:      60,
			TotalEpisodeCount: 100,
			SizeOnDisk:        10737418240,
			PercentOfEpisodes: 50.0,
		},
		Seasons: []Season{
			{
				SeasonNumber: 1,
				Monitored:    true,
				Statistics: Statistics{
					EpisodeFileCount:  10,
					EpisodeCount:      12,
					TotalEpisodeCount: 12,
					SizeOnDisk:        2147483648,
					PercentOfEpisodes: 83.3,
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if !containsString(r.URL.Path, "/api/v3/series/123") {
			t.Errorf("expected path to contain /api/v3/series/123, got %s", r.URL.Path)
		}

		if r.Header.Get("X-Api-Key") != "testkey" {
			t.Errorf("expected X-Api-Key header")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockSeries)
	}))
	defer server.Close()

	client := NewSonarrClient(ClientConfig{
		Name:    "sonarr",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	series, err := client.GetSeries(context.Background(), 123)
	if err != nil {
		t.Fatalf("GetSeries failed: %v", err)
	}

	if series.ID != mockSeries.ID {
		t.Errorf("ID = %d, want %d", series.ID, mockSeries.ID)
	}

	if series.Title != mockSeries.Title {
		t.Errorf("Title = %q, want %q", series.Title, mockSeries.Title)
	}

	if series.SeasonCount != mockSeries.SeasonCount {
		t.Errorf("SeasonCount = %d, want %d", series.SeasonCount, mockSeries.SeasonCount)
	}

	if !series.Monitored {
		t.Error("expected Monitored to be true")
	}

	if len(series.Seasons) != 1 {
		t.Errorf("expected 1 season, got %d", len(series.Seasons))
	}
}

func TestSonarrGetAllSeries(t *testing.T) {
	mockSeriesList := []Series{
		{
			ID:          1,
			Title:       "Series One",
			SeasonCount: 3,
			Status:      "ended",
			Monitored:   true,
		},
		{
			ID:          2,
			Title:       "Series Two",
			SeasonCount: 5,
			Status:      "continuing",
			Monitored:   false,
		},
		{
			ID:          3,
			Title:       "Series Three",
			SeasonCount: 10,
			Status:      "continuing",
			Monitored:   true,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if !containsString(r.URL.Path, "/api/v3/series") {
			t.Errorf("expected path to contain /api/v3/series, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockSeriesList)
	}))
	defer server.Close()

	client := NewSonarrClient(ClientConfig{
		Name:    "sonarr",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	seriesList, err := client.GetAllSeries(context.Background())
	if err != nil {
		t.Fatalf("GetAllSeries failed: %v", err)
	}

	if len(seriesList) != 3 {
		t.Fatalf("expected 3 series, got %d", len(seriesList))
	}

	if seriesList[0].Title != "Series One" {
		t.Errorf("first series title = %q, want %q", seriesList[0].Title, "Series One")
	}

	if seriesList[1].Title != "Series Two" {
		t.Errorf("second series title = %q, want %q", seriesList[1].Title, "Series Two")
	}

	if seriesList[2].Title != "Series Three" {
		t.Errorf("third series title = %q, want %q", seriesList[2].Title, "Series Three")
	}
}

func TestSonarrGetEpisodes(t *testing.T) {
	mockEpisodes := []Episode{
		{
			ID:            1001,
			SeriesID:      123,
			EpisodeFileID: 5001,
			SeasonNumber:  1,
			EpisodeNumber: 1,
			Title:         "Pilot",
			AirDate:       "2023-01-01",
			HasFile:       true,
			Monitored:     true,
		},
		{
			ID:            1002,
			SeriesID:      123,
			EpisodeFileID: 0,
			SeasonNumber:  1,
			EpisodeNumber: 2,
			Title:         "Episode Two",
			AirDate:       "2023-01-08",
			HasFile:       false,
			Monitored:     true,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if !containsString(r.URL.Path, "/api/v3/episode") {
			t.Errorf("expected path to contain /api/v3/episode, got %s", r.URL.Path)
		}

		if r.URL.Query().Get("seriesId") != "123" {
			t.Errorf("expected seriesId=123, got %s", r.URL.Query().Get("seriesId"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockEpisodes)
	}))
	defer server.Close()

	client := NewSonarrClient(ClientConfig{
		Name:    "sonarr",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	episodes, err := client.GetEpisodes(context.Background(), 123)
	if err != nil {
		t.Fatalf("GetEpisodes failed: %v", err)
	}

	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(episodes))
	}

	if episodes[0].Title != "Pilot" {
		t.Errorf("first episode title = %q, want %q", episodes[0].Title, "Pilot")
	}

	if !episodes[0].HasFile {
		t.Error("expected first episode to have file")
	}

	if episodes[1].HasFile {
		t.Error("expected second episode to not have file")
	}
}

func TestSonarrSearchEpisodes(t *testing.T) {
	episodeIDs := []int{101, 102, 103}
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if !containsString(r.URL.Path, "/api/v3/command") {
			t.Errorf("expected path to contain /api/v3/command, got %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     1,
			"status": "queued",
		})
	}))
	defer server.Close()

	client := NewSonarrClient(ClientConfig{
		Name:    "sonarr",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	err := client.SearchEpisodes(context.Background(), episodeIDs)
	if err != nil {
		t.Fatalf("SearchEpisodes failed: %v", err)
	}

	if receivedBody["name"] != "EpisodeSearch" {
		t.Errorf("command name = %q, want %q", receivedBody["name"], "EpisodeSearch")
	}

	// Verify episodeIds in the request
	if receivedBody["episodeIds"] == nil {
		t.Fatal("expected episodeIds in request body")
	}
}

func TestSonarrSearchSeason(t *testing.T) {
	tests := []struct {
		name         string
		seriesID     int
		seasonNumber int
	}{
		{
			name:         "search season 1",
			seriesID:     123,
			seasonNumber: 1,
		},
		{
			name:         "search season 5",
			seriesID:     456,
			seasonNumber: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody map[string]interface{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}

				if !containsString(r.URL.Path, "/api/v3/command") {
					t.Errorf("expected path to contain /api/v3/command, got %s", r.URL.Path)
				}

				if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
					t.Fatalf("failed to decode request body: %v", err)
				}

				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"id":     1,
					"status": "queued",
				})
			}))
			defer server.Close()

			client := NewSonarrClient(ClientConfig{
				Name:    "sonarr",
				BaseURL: server.URL,
				APIKey:  "testkey",
			})

			err := client.SearchSeason(context.Background(), tt.seriesID, tt.seasonNumber)
			if err != nil {
				t.Fatalf("SearchSeason failed: %v", err)
			}

			if receivedBody["name"] != "SeasonSearch" {
				t.Errorf("command name = %q, want %q", receivedBody["name"], "SeasonSearch")
			}

			seriesIDFloat, ok := receivedBody["seriesId"].(float64)
			if !ok || int(seriesIDFloat) != tt.seriesID {
				t.Errorf("seriesId = %v, want %d", receivedBody["seriesId"], tt.seriesID)
			}

			seasonNumFloat, ok := receivedBody["seasonNumber"].(float64)
			if !ok || int(seasonNumFloat) != tt.seasonNumber {
				t.Errorf("seasonNumber = %v, want %d", receivedBody["seasonNumber"], tt.seasonNumber)
			}
		})
	}
}

func TestSonarrDeleteSeries(t *testing.T) {
	tests := []struct {
		name        string
		seriesID    int
		deleteFiles bool
	}{
		{
			name:        "delete without files",
			seriesID:    123,
			deleteFiles: false,
		},
		{
			name:        "delete with files",
			seriesID:    456,
			deleteFiles: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Errorf("expected DELETE, got %s", r.Method)
				}

				if !containsString(r.URL.Path, "/api/v3/series/") {
					t.Errorf("expected path to contain /api/v3/series/, got %s", r.URL.Path)
				}

				deleteFilesParam := r.URL.Query().Get("deleteFiles")
				expectedParam := "false"
				if tt.deleteFiles {
					expectedParam = "true"
				}

				if deleteFilesParam != expectedParam {
					t.Errorf("deleteFiles = %s, want %s", deleteFilesParam, expectedParam)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := NewSonarrClient(ClientConfig{
				Name:    "sonarr",
				BaseURL: server.URL,
				APIKey:  "testkey",
			})

			err := client.DeleteSeries(context.Background(), tt.seriesID, tt.deleteFiles)
			if err != nil {
				t.Fatalf("DeleteSeries failed: %v", err)
			}
		})
	}
}
