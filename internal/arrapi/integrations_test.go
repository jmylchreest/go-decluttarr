package arrapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestIntegrationContracts(t *testing.T) {
	for _, tc := range []struct {
		kind, version, status, entity string
		count                         int
	}{
		{"sportarr", "v3", "/api/system/status", "/api/events/99", 2},
		{"bookshelf", "v1", "/api/v1/system/status", "/api/v1/author/99", 1},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			fixture, err := os.ReadFile("testdata/" + tc.kind + "-queue.json")
			if err != nil {
				t.Fatal(err)
			}
			deleted := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Api-Key") != "test-key" {
					t.Error("missing API key")
				}
				switch r.URL.Path {
				case "/api/" + tc.version + "/queue":
					if r.Method != "GET" {
						t.Error("unexpected mutation")
					}
					w.Write(fixture)
				case tc.status:
					fmt.Fprintf(w, `{"appName":%q,"version":"test"}`, tc.kind)
				case tc.entity:
					fmt.Fprint(w, `{"monitored":true}`)
				case "/api/" + tc.version + "/queue/41":
					if r.Method != "DELETE" {
						t.Error("wrong method")
					}
					for _, key := range []string{"removeFromClient", "blocklist", "skipRedownload"} {
						if r.URL.Query().Get(key) != "false" {
							t.Errorf("%s not explicitly false", key)
						}
					}
					deleted = true
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			client := NewClient(ClientConfig{BaseURL: server.URL, APIKey: "test-key", APIVersion: tc.version, AppType: tc.kind})
			defer client.Close()
			queue, err := client.GetQueue(context.Background())
			if err != nil || len(queue) != tc.count {
				t.Fatalf("queue=%v err=%v", queue, err)
			}
			if tc.kind == "sportarr" && (queue[0].EpisodeID == nil || *queue[0].EpisodeID != 99) {
				t.Fatal("event identity lost")
			}
			if tc.kind == "bookshelf" && (queue[0].Sizeleft != 456.5 || queue[0].AuthorID == nil) {
				t.Fatal("book queue decode mismatch")
			}
			if _, err := client.GetSystemStatus(context.Background()); err != nil {
				t.Fatal(err)
			}
			entity := "author"
			if tc.kind == "sportarr" {
				entity = "event"
			}
			if monitored, err := client.GetMonitoredStatus(context.Background(), entity, 99); err != nil || !monitored {
				t.Fatalf("monitored=%v err=%v", monitored, err)
			}
			if err := client.DeleteQueueItem(context.Background(), 41, DeleteOptions{}); err != nil || !deleted {
				t.Fatalf("delete=%v err=%v", deleted, err)
			}
		})
	}
}
