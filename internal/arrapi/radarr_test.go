package arrapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRadarrGetMovie(t *testing.T) {
	mockMovie := Movie{
		ID:                  123,
		Title:               "Test Movie",
		OriginalTitle:       "Test Movie Original",
		Year:                2023,
		Status:              "released",
		Overview:            "A test movie",
		Path:                "/movies/test-movie",
		Monitored:           true,
		Added:               time.Now(),
		HasFile:             true,
		SizeOnDisk:          5368709120,
		Runtime:             120,
		MinimumAvailability: "released",
		IsAvailable:         true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if !containsString(r.URL.Path, "/api/v3/movie/123") {
			t.Errorf("expected path to contain /api/v3/movie/123, got %s", r.URL.Path)
		}

		if r.Header.Get("X-Api-Key") != "testkey" {
			t.Errorf("expected X-Api-Key header")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockMovie)
	}))
	defer server.Close()

	client := NewRadarrClient(ClientConfig{
		Name:    "radarr",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	movie, err := client.GetMovie(context.Background(), 123)
	if err != nil {
		t.Fatalf("GetMovie failed: %v", err)
	}

	if movie.ID != mockMovie.ID {
		t.Errorf("ID = %d, want %d", movie.ID, mockMovie.ID)
	}

	if movie.Title != mockMovie.Title {
		t.Errorf("Title = %q, want %q", movie.Title, mockMovie.Title)
	}

	if movie.Year != mockMovie.Year {
		t.Errorf("Year = %d, want %d", movie.Year, mockMovie.Year)
	}

	if !movie.Monitored {
		t.Error("expected Monitored to be true")
	}

	if !movie.HasFile {
		t.Error("expected HasFile to be true")
	}

	if movie.Runtime != 120 {
		t.Errorf("Runtime = %d, want %d", movie.Runtime, 120)
	}
}

func TestRadarrGetAllMovies(t *testing.T) {
	mockMoviesList := []Movie{
		{
			ID:        1,
			Title:     "Movie One",
			Year:      2020,
			Status:    "released",
			Monitored: true,
			HasFile:   true,
		},
		{
			ID:        2,
			Title:     "Movie Two",
			Year:      2021,
			Status:    "released",
			Monitored: false,
			HasFile:   false,
		},
		{
			ID:        3,
			Title:     "Movie Three",
			Year:      2022,
			Status:    "announced",
			Monitored: true,
			HasFile:   false,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if !containsString(r.URL.Path, "/api/v3/movie") {
			t.Errorf("expected path to contain /api/v3/movie, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockMoviesList)
	}))
	defer server.Close()

	client := NewRadarrClient(ClientConfig{
		Name:    "radarr",
		BaseURL: server.URL,
		APIKey:  "testkey",
	})

	moviesList, err := client.GetAllMovies(context.Background())
	if err != nil {
		t.Fatalf("GetAllMovies failed: %v", err)
	}

	if len(moviesList) != 3 {
		t.Fatalf("expected 3 movies, got %d", len(moviesList))
	}

	if moviesList[0].Title != "Movie One" {
		t.Errorf("first movie title = %q, want %q", moviesList[0].Title, "Movie One")
	}

	if moviesList[1].Title != "Movie Two" {
		t.Errorf("second movie title = %q, want %q", moviesList[1].Title, "Movie Two")
	}

	if moviesList[2].Title != "Movie Three" {
		t.Errorf("third movie title = %q, want %q", moviesList[2].Title, "Movie Three")
	}

	if moviesList[0].Year != 2020 {
		t.Errorf("first movie year = %d, want %d", moviesList[0].Year, 2020)
	}

	if moviesList[1].Monitored {
		t.Error("expected second movie to not be monitored")
	}
}

func TestRadarrSearchMovie(t *testing.T) {
	tests := []struct {
		name    string
		movieID int
	}{
		{
			name:    "search movie 123",
			movieID: 123,
		},
		{
			name:    "search movie 456",
			movieID: 456,
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

			client := NewRadarrClient(ClientConfig{
				Name:    "radarr",
				BaseURL: server.URL,
				APIKey:  "testkey",
			})

			err := client.SearchMovie(context.Background(), tt.movieID)
			if err != nil {
				t.Fatalf("SearchMovie failed: %v", err)
			}

			if receivedBody["name"] != "MoviesSearch" {
				t.Errorf("command name = %q, want %q", receivedBody["name"], "MoviesSearch")
			}

			// Verify movieIds in the request
			if receivedBody["movieIds"] == nil {
				t.Fatal("expected movieIds in request body")
			}

			movieIDs, ok := receivedBody["movieIds"].([]interface{})
			if !ok {
				t.Fatal("movieIds is not an array")
			}

			if len(movieIDs) != 1 {
				t.Errorf("expected 1 movie ID, got %d", len(movieIDs))
			}

			movieIDFloat, ok := movieIDs[0].(float64)
			if !ok || int(movieIDFloat) != tt.movieID {
				t.Errorf("movieId = %v, want %d", movieIDs[0], tt.movieID)
			}
		})
	}
}

func TestRadarrDeleteMovie(t *testing.T) {
	tests := []struct {
		name        string
		movieID     int
		deleteFiles bool
	}{
		{
			name:        "delete without files",
			movieID:     123,
			deleteFiles: false,
		},
		{
			name:        "delete with files",
			movieID:     456,
			deleteFiles: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Errorf("expected DELETE, got %s", r.Method)
				}

				if !containsString(r.URL.Path, "/api/v3/movie/") {
					t.Errorf("expected path to contain /api/v3/movie/, got %s", r.URL.Path)
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

			client := NewRadarrClient(ClientConfig{
				Name:    "radarr",
				BaseURL: server.URL,
				APIKey:  "testkey",
			})

			err := client.DeleteMovie(context.Background(), tt.movieID, tt.deleteFiles)
			if err != nil {
				t.Fatalf("DeleteMovie failed: %v", err)
			}
		})
	}
}

func TestRadarrErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		wantErrContains string
	}{
		{
			name:            "404 movie not found",
			statusCode:      http.StatusNotFound,
			responseBody:    `{"error": "Movie not found"}`,
			wantErrContains: "404",
		},
		{
			name:            "401 unauthorized",
			statusCode:      http.StatusUnauthorized,
			responseBody:    `{"error": "Invalid API key"}`,
			wantErrContains: "401",
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

			client := NewRadarrClient(ClientConfig{
				Name:    "radarr",
				BaseURL: server.URL,
				APIKey:  "testkey",
			})

			_, err := client.GetMovie(context.Background(), 123)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !containsString(err.Error(), tt.wantErrContains) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}
