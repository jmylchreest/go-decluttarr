package jobs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmylchreest/go-decluttarr/internal/arrapi"
	"github.com/jmylchreest/go-decluttarr/internal/config"
)

func BenchmarkGetAllQueues(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewManager(&config.Config{}, logger, "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		fmt.Fprint(w, `{"page":1,"pageSize":1000,"totalRecords":1,"records":[{"id":1,"downloadId":"abc"}]}`)
	}))
	defer server.Close()
	for i := 0; i < 8; i++ {
		m.RegisterArrClient(fmt.Sprint(i), arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL, Logger: logger}))
	}
	defer m.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queues, err := m.GetAllQueues(context.Background())
		if err != nil || len(queues) != 8 {
			b.Fatalf("queues=%d err=%v", len(queues), err)
		}
	}
}

func TestQueueFetchConcurrencyAndPartialFailures(t *testing.T) {
	var active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		if r.Header.Get("X-Api-Key") == "fail" {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"totalRecords":0,"records":[]}`)
	}))
	defer server.Close()
	m := NewManager(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), "")
	defer m.Close()
	for i := 0; i < 10; i++ {
		key := "ok"
		if i == 9 {
			key = "fail"
		}
		m.RegisterArrClient(fmt.Sprint(i), arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL, APIKey: key}))
	}
	queues, err := m.GetAllQueues(context.Background())
	if err == nil || len(queues) != 9 {
		t.Fatalf("queues=%d err=%v", len(queues), err)
	}
	if peak.Load() > 4 {
		t.Fatalf("concurrency=%d", peak.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.GetAllQueues(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestIgnoredClientsDoNotHideQueueOwnership(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"totalRecords":1,"records":[{"downloadId":"owned","downloadClient":"ignored"}]}`)
	}))
	defer server.Close()
	cfg := &config.Config{General: config.GeneralConfig{IgnoreDownloadClients: []string{"ignored"}}}
	m := NewManager(cfg, nil, "")
	m.RegisterArrClient("arr", arrapi.NewClient(arrapi.ClientConfig{BaseURL: server.URL}))
	defer m.Close()
	m.RegisterDownloadClient("ignored", nil)
	if len(m.GetAllDownloadClients()) != 0 || !m.IsIgnoredDownloadClient("ignored") {
		t.Fatal("ignored client included")
	}
	queues, err := m.GetAllQueues(context.Background())
	if err != nil || len(queues["arr"]) != 1 {
		t.Fatalf("ownership hidden: %v %v", queues, err)
	}
}
