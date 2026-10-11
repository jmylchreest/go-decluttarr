package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/jobs"
)

func TestRegisterNewIntegrations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected mutation: %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1/queue", "/api/v3/queue":
			fmt.Fprint(w, `{"totalRecords":1,"records":[{"id":1,"downloadId":"owned"}]}`)
		case "/api/v1/requests":
			fmt.Fprint(w, `{"requests":[{"id":1,"status":"processing"}]}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	cfg := &config.Config{Instances: config.InstancesConfig{
		Sportarr:  []config.InstanceConfig{{Name: "sports", URL: server.URL, APIKey: "test"}},
		Bookshelf: []config.InstanceConfig{{Name: "books", URL: server.URL, APIKey: "test"}},
		Shelfarr:  []config.ShelfarrConfig{{Name: "requests", URL: server.URL, APIKey: "test", ProtectedCategories: []string{"shelfarr"}}},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := jobs.NewManager(cfg, logger, "")
	defer m.Close()
	registerAllJobs(m, cfg, logger)
	queues, err := m.GetAllQueues(context.Background())
	if err != nil || len(queues) != 2 || len(queues["sports"]) != 1 || len(queues["books"]) != 1 {
		t.Fatalf("queues=%v err=%v", queues, err)
	}
	if _, exists := queues["requests"]; exists {
		t.Fatal("Shelfarr treated as ownership queue")
	}
	if err := m.RunAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := m.GetLastStats().Shelfarr["requests"]
	if snapshot == nil || snapshot.StatusCounts["processing"] != 1 {
		t.Fatalf("snapshot=%v", snapshot)
	}
}
