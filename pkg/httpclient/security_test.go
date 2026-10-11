package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCredentialsNeverFollowRedirects(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	c := New(DefaultConfig())
	defer c.Close()
	req, _ := http.NewRequest(http.MethodGet, source.URL+"?apikey=secret-value", nil)
	req.Header.Set("X-Api-Key", "secret-value")
	resp, err := c.Do(context.Background(), req)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || reached {
		t.Fatal("followed authenticated redirect")
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Fatal("API key in error")
	}
}

func TestTransportErrorRedactsAPIKey(t *testing.T) {
	c := New(DefaultConfig())
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Get(ctx, "http://localhost:1/api?apikey=secret-value")
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestErrorBodyIsBounded(t *testing.T) {
	c := New(DefaultConfig())
	resp := &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 1000000)))}
	err := c.DecodeJSON(resp, nil)
	if err == nil || len(err.Error()) > 4200 {
		t.Fatal("unbounded error response")
	}
}
