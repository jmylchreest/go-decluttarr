package downloadclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNZBGetNewClient(t *testing.T) {
	tests := []struct {
		name   string
		config NZBGetConfig
	}{
		{
			name: "valid config with all fields",
			config: NZBGetConfig{
				BaseURL:  "http://localhost:6789",
				Username: "nzbget",
				Password: "tegbzn6789",
				Timeout:  30 * time.Second,
			},
		},
		{
			name: "valid config with default timeout",
			config: NZBGetConfig{
				BaseURL:  "http://localhost:6789",
				Username: "nzbget",
				Password: "tegbzn6789",
			},
		},
		{
			name: "valid config without auth",
			config: NZBGetConfig{
				BaseURL: "http://localhost:6789",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewNZBGetClient(tt.config)
			require.NotNil(t, client)
			assert.Equal(t, "NZBGet", client.Name())
			assert.NotNil(t, client.http)
			assert.NotNil(t, client.logger)
			assert.Equal(t, tt.config.BaseURL, client.baseURL)
			assert.Equal(t, tt.config.Username, client.username)
			assert.Equal(t, tt.config.Password, client.password)
		})
	}
}

func TestNZBGetListGroups(t *testing.T) {
	mockGroups := []NZBGetGroup{
		{
			NZBID:           12345,
			NZBName:         "Test.Download.S01E01.1080p",
			Status:          "DOWNLOADING",
			FileSizeMB:      1024,
			RemainingSizeMB: 256,
			Health:          1000,
			Category:        "tv",
		},
		{
			NZBID:           12346,
			NZBName:         "Another.Download.2023",
			Status:          "PAUSED",
			FileSizeMB:      2048,
			RemainingSizeMB: 2048,
			Health:          1000,
			Category:        "movies",
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
			name: "successful listgroups call",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/jsonrpc", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Verify Basic Auth
				username, password, ok := r.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "nzbget", username)
				assert.Equal(t, "password", password)

				// Parse and verify RPC request
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var req rpcRequest
				err = json.Unmarshal(body, &req)
				require.NoError(t, err)
				assert.Equal(t, "listgroups", req.Method)
				assert.Equal(t, "1.1", req.Version)

				// Send RPC response
				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(mustMarshal(mockGroups)),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "empty groups list",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`[]`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "rpc error response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				errMsg := "Method not found"
				resp := rpcResponse{
					Version: "1.1",
					Error:   &errMsg,
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr:     true,
			errContains: "RPC error",
		},
		{
			name: "http error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr:     true,
			errContains: "RPC call failed with HTTP 401",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := NZBGetConfig{
				BaseURL:  server.URL,
				Username: "nzbget",
				Password: "password",
			}

			client := NewNZBGetClient(cfg)
			groups, err := client.GetQueue(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, groups, tt.wantCount)

				if tt.wantCount > 0 {
					assert.Equal(t, 12345, groups[0].NZBID)
					assert.Equal(t, "Test.Download.S01E01.1080p", groups[0].NZBName)
					assert.Equal(t, "DOWNLOADING", groups[0].Status)
					assert.Equal(t, "tv", groups[0].Category)
					assert.Equal(t, 1024, groups[0].FileSizeMB)
					assert.Equal(t, 256, groups[0].RemainingSizeMB)
				}
			}
		})
	}
}

func TestNZBGetHistory(t *testing.T) {
	mockHistory := []NZBGetHistoryItem{
		{
			NZBID:      54321,
			NZBName:    "Completed.Download.mkv",
			Status:     "SUCCESS",
			Category:   "movies",
			FileSizeMB: 1536,
		},
		{
			NZBID:      54322,
			NZBName:    "Failed.Download.mkv",
			Status:     "FAILURE",
			Category:   "tv",
			FileSizeMB: 2048,
		},
	}

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantCount      int
		wantErr        bool
	}{
		{
			name: "successful history call",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req rpcRequest
				_ = json.Unmarshal(body, &req)
				assert.Equal(t, "history", req.Method)

				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(mustMarshal(mockHistory)),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "empty history",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`[]`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := NZBGetConfig{
				BaseURL:  server.URL,
				Username: "nzbget",
				Password: "password",
			}

			client := NewNZBGetClient(cfg)
			history, err := client.GetHistory(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, history, tt.wantCount)

				if tt.wantCount > 0 {
					assert.Equal(t, 54321, history[0].NZBID)
					assert.Equal(t, "Completed.Download.mkv", history[0].NZBName)
					assert.Equal(t, "SUCCESS", history[0].Status)
				}
			}
		})
	}
}

func TestNZBGetEditQueue(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		nzbID          int
		serverResponse func(w http.ResponseWriter, r *http.Request)
		callFunc       func(client *NZBGetClient, ctx context.Context, nzbID int) error
		wantErr        bool
		errContains    string
	}{
		{
			name:   "successful delete",
			method: "GroupFinalDelete",
			nzbID:  12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req rpcRequest
				_ = json.Unmarshal(body, &req)

				assert.Equal(t, "editqueue", req.Method)
				assert.Len(t, req.Params, 4)
				assert.Equal(t, "GroupFinalDelete", req.Params[0])
				assert.Equal(t, float64(0), req.Params[1])
				assert.Equal(t, "", req.Params[2])

				// Verify NZB ID array
				ids := req.Params[3].([]interface{})
				assert.Len(t, ids, 1)
				assert.Equal(t, float64(12345), ids[0])

				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`true`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			callFunc: func(client *NZBGetClient, ctx context.Context, nzbID int) error {
				return client.DeleteItem(ctx, nzbID)
			},
			wantErr: false,
		},
		{
			name:   "delete operation failed",
			method: "GroupFinalDelete",
			nzbID:  12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`false`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			callFunc: func(client *NZBGetClient, ctx context.Context, nzbID int) error {
				return client.DeleteItem(ctx, nzbID)
			},
			wantErr:     true,
			errContains: "delete operation failed",
		},
		{
			name:   "successful pause",
			method: "GroupPause",
			nzbID:  12346,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req rpcRequest
				_ = json.Unmarshal(body, &req)

				assert.Equal(t, "editqueue", req.Method)
				assert.Equal(t, "GroupPause", req.Params[0])

				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`true`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			callFunc: func(client *NZBGetClient, ctx context.Context, nzbID int) error {
				return client.PauseItem(ctx, nzbID)
			},
			wantErr: false,
		},
		{
			name:   "pause operation failed",
			method: "GroupPause",
			nzbID:  12346,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`false`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			callFunc: func(client *NZBGetClient, ctx context.Context, nzbID int) error {
				return client.PauseItem(ctx, nzbID)
			},
			wantErr:     true,
			errContains: "pause operation failed",
		},
		{
			name:   "successful resume",
			method: "GroupResume",
			nzbID:  12347,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req rpcRequest
				_ = json.Unmarshal(body, &req)

				assert.Equal(t, "editqueue", req.Method)
				assert.Equal(t, "GroupResume", req.Params[0])

				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`true`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			callFunc: func(client *NZBGetClient, ctx context.Context, nzbID int) error {
				return client.ResumeItem(ctx, nzbID)
			},
			wantErr: false,
		},
		{
			name:   "resume operation failed",
			method: "GroupResume",
			nzbID:  12347,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`false`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			callFunc: func(client *NZBGetClient, ctx context.Context, nzbID int) error {
				return client.ResumeItem(ctx, nzbID)
			},
			wantErr:     true,
			errContains: "resume operation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := NZBGetConfig{
				BaseURL:  server.URL,
				Username: "nzbget",
				Password: "password",
			}

			client := NewNZBGetClient(cfg)
			err := tt.callFunc(client, context.Background(), tt.nzbID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNZBGetRPCCall(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		params         []any
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		errContains    string
	}{
		{
			name:   "successful rpc call",
			method: "version",
			params: []any{},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`"21.1"`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:   "rpc call with params",
			method: "rate",
			params: []any{1000},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req rpcRequest
				_ = json.Unmarshal(body, &req)

				assert.Equal(t, "rate", req.Method)
				assert.Len(t, req.Params, 1)
				assert.Equal(t, float64(1000), req.Params[0])

				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`true`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:   "malformed json response",
			method: "test",
			params: []any{},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{invalid json}`))
			},
			wantErr:     true,
			errContains: "failed to decode RPC response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := NZBGetConfig{
				BaseURL:  server.URL,
				Username: "nzbget",
				Password: "password",
			}

			client := NewNZBGetClient(cfg)

			var result interface{}
			err := client.rpcCall(context.Background(), tt.method, tt.params, &result)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNZBGetContextCancellation(t *testing.T) {
	// Server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		resp := rpcResponse{
			Version: "1.1",
			Result:  json.RawMessage(`[]`),
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := NZBGetConfig{
		BaseURL:  server.URL,
		Username: "nzbget",
		Password: "password",
		Timeout:  10 * time.Millisecond, // Very short timeout
	}

	client := NewNZBGetClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.GetQueue(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestNZBGetBasicAuth(t *testing.T) {
	tests := []struct {
		name         string
		username     string
		password     string
		expectAuth   bool
		expectUser   string
		expectPass   string
	}{
		{
			name:       "with credentials",
			username:   "testuser",
			password:   "testpass",
			expectAuth: true,
			expectUser: "testuser",
			expectPass: "testpass",
		},
		{
			name:       "without credentials",
			username:   "",
			password:   "",
			expectAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()

				if tt.expectAuth {
					assert.True(t, ok)
					assert.Equal(t, tt.expectUser, username)
					assert.Equal(t, tt.expectPass, password)
				} else {
					assert.False(t, ok)
				}

				resp := rpcResponse{
					Version: "1.1",
					Result:  json.RawMessage(`[]`),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			cfg := NZBGetConfig{
				BaseURL:  server.URL,
				Username: tt.username,
				Password: tt.password,
			}

			client := NewNZBGetClient(cfg)
			_, err := client.GetQueue(context.Background())
			assert.NoError(t, err)
		})
	}
}

// Helper function to marshal JSON without error
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
