package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	cfg := DefaultConfig()
	client := New(cfg)

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.timeout != cfg.Timeout {
		t.Errorf("expected timeout %v, got %v", cfg.Timeout, client.timeout)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.MaxIdleConns != 10 {
		t.Errorf("expected MaxIdleConns 10, got %d", cfg.MaxIdleConns)
	}
	if cfg.IdleConnTimeout != 90*time.Second {
		t.Errorf("expected IdleConnTimeout 90s, got %v", cfg.IdleConnTimeout)
	}
	if cfg.SkipTLSVerify != false {
		t.Error("expected SkipTLSVerify false")
	}
}

func TestClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	client := New(DefaultConfig())
	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestClientPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("expected Content-Type text/plain, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := New(DefaultConfig())
	resp, err := client.Post(context.Background(), server.URL, "text/plain", nil)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}
}

func TestClientPostJSON(t *testing.T) {
	type TestPayload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	expectedPayload := TestPayload{Name: "test", Value: 42}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var received TestPayload
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if received.Name != expectedPayload.Name || received.Value != expectedPayload.Value {
			t.Errorf("expected %+v, got %+v", expectedPayload, received)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}))
	defer server.Close()

	client := New(DefaultConfig())
	resp, err := client.PostJSON(context.Background(), server.URL, expectedPayload)
	if err != nil {
		t.Fatalf("PostJSON failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestClientDecodeJSON(t *testing.T) {
	type Response struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response{Message: "success", Code: 100})
	}))
	defer server.Close()

	client := New(DefaultConfig())
	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	var result Response
	if err := client.DecodeJSON(resp, &result); err != nil {
		t.Fatalf("DecodeJSON failed: %v", err)
	}

	if result.Message != "success" || result.Code != 100 {
		t.Errorf("expected {success, 100}, got {%s, %d}", result.Message, result.Code)
	}
}

func TestClientDecodeJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := New(DefaultConfig())
	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	var result map[string]interface{}
	err = client.DecodeJSON(resp, &result)
	if err == nil {
		t.Fatal("expected error from DecodeJSON on non-2xx response")
	}
}

func TestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Timeout = 50 * time.Millisecond
	client := New(cfg)

	ctx := context.Background()
	_, err := client.Get(ctx, server.URL)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClientContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(DefaultConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Get(ctx, server.URL)
	if err == nil {
		t.Fatal("expected context timeout error")
	}
}
