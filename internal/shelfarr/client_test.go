package shelfarr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantErr    bool
	}{
		{"states", `{"requests":[{"id":1,"status":"processing","attention_needed":true},{"id":2,"status":"future_state"}]}`, 200, false},
		{"empty", `{"requests":[]}`, 200, false},
		{"missing", `{}`, 200, true}, {"null", `{"requests":null}`, 200, true},
		{"invalid record", `{"requests":[{"id":1}]}`, 200, true},
		{"unauthorized", `{}`, 401, true}, {"unavailable", `{}`, 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/shelfarr/api/v1/requests" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" || r.URL.Query().Get("limit") != "100" {
					t.Error("auth/limit mismatch")
				}
				if r.Header.Get("X-Api-Key") != "" {
					t.Error("arr authentication used")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c := NewClient(Config{BaseURL: server.URL + "/shelfarr/", APIKey: "test-token"})
			defer c.Close()
			snapshot, err := c.GetSnapshot(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if tc.name == "states" && (snapshot.Observed != 2 || snapshot.AttentionNeeded != 1 || snapshot.StatusCounts["future_state"] != 1) {
				t.Fatalf("snapshot=%+v", snapshot)
			}
		})
	}
}

func TestSnapshotMarksLimitAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests := make([]Request, RequestLimit)
		for i := range requests {
			requests[i] = Request{ID: i + 1, Status: "downloading"}
		}
		json.NewEncoder(w).Encode(map[string]any{"requests": requests})
	}))
	defer server.Close()
	c := NewClient(Config{BaseURL: server.URL})
	defer c.Close()
	snapshot, err := c.GetSnapshot(context.Background())
	if err != nil || !snapshot.AtLimit {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.GetSnapshot(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
}
