package strikes

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name        string
		persistPath string
		wantPath    string
	}{
		{
			name:        "with persist path",
			persistPath: "/tmp/strikes.json",
			wantPath:    "/tmp/strikes.json",
		},
		{
			name:        "without persist path",
			persistPath: "",
			wantPath:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(tt.persistPath, nil)
			if h == nil {
				t.Fatal("expected non-nil handler")
			}
			if h.persistPath != tt.wantPath {
				t.Errorf("expected persistPath %s, got %s", tt.wantPath, h.persistPath)
			}
			if h.strikes == nil {
				t.Error("expected strikes map to be initialized")
			}
		})
	}
}

func TestAdd(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	tests := []struct {
		name       string
		downloadID string
		job        string
		itemName   string
		wantCount  int
	}{
		{
			name:       "first strike",
			downloadID: "dl1",
			job:        "job1",
			itemName:   "item1",
			wantCount:  1,
		},
		{
			name:       "second strike same ID",
			downloadID: "dl1",
			job:        "job1",
			itemName:   "item1",
			wantCount:  2,
		},
		{
			name:       "third strike same ID",
			downloadID: "dl1",
			job:        "job1",
			itemName:   "item1",
			wantCount:  3,
		},
		{
			name:       "different ID",
			downloadID: "dl2",
			job:        "job2",
			itemName:   "item2",
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := h.Add(tt.downloadID, tt.job, tt.itemName)
			if count != tt.wantCount {
				t.Errorf("expected count %d, got %d", tt.wantCount, count)
			}
		})
	}

	// Verify cycle counter
	if h.strikesAdded != 4 {
		t.Errorf("expected strikesAdded 4, got %d", h.strikesAdded)
	}
}

func TestGet(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Add some strikes
	h.Add("dl1", "job1", "item1")
	h.Add("dl1", "job1", "item1")
	h.Add("dl2", "job2", "item2")

	tests := []struct {
		name       string
		downloadID string
		wantCount  int
	}{
		{
			name:       "known ID with 2 strikes",
			downloadID: "dl1",
			wantCount:  2,
		},
		{
			name:       "known ID with 1 strike",
			downloadID: "dl2",
			wantCount:  1,
		},
		{
			name:       "unknown ID",
			downloadID: "dl3",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := h.Get(tt.downloadID)
			if count != tt.wantCount {
				t.Errorf("expected count %d, got %d", tt.wantCount, count)
			}
		})
	}
}

func TestHasExceeded(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Add strikes
	h.Add("dl1", "job1", "item1") // 1
	h.Add("dl1", "job1", "item1") // 2
	h.Add("dl1", "job1", "item1") // 3

	tests := []struct {
		name        string
		downloadID  string
		maxStrikes  int
		wantExceed  bool
	}{
		{
			name:       "below threshold",
			downloadID: "dl1",
			maxStrikes: 5,
			wantExceed: false,
		},
		{
			name:       "at threshold",
			downloadID: "dl1",
			maxStrikes: 3,
			wantExceed: true,
		},
		{
			name:       "above threshold",
			downloadID: "dl1",
			maxStrikes: 2,
			wantExceed: true,
		},
		{
			name:       "unknown ID",
			downloadID: "dl2",
			maxStrikes: 1,
			wantExceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exceeded := h.HasExceeded(tt.downloadID, tt.maxStrikes)
			if exceeded != tt.wantExceed {
				t.Errorf("expected exceeded %v, got %v", tt.wantExceed, exceeded)
			}
		})
	}
}

func TestReset(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Add strikes
	h.Add("dl1", "job1", "item1")
	h.Add("dl1", "job1", "item1")
	h.Add("dl2", "job2", "item2")

	// Reset dl1
	h.Reset("dl1")

	if count := h.Get("dl1"); count != 0 {
		t.Errorf("expected count 0 after reset, got %d", count)
	}

	if count := h.Get("dl2"); count != 1 {
		t.Errorf("expected dl2 count to remain 1, got %d", count)
	}

	if h.strikesReset != 1 {
		t.Errorf("expected strikesReset 1, got %d", h.strikesReset)
	}

	// Reset unknown ID (should not panic)
	h.Reset("dl3")
	if h.strikesReset != 1 {
		t.Errorf("expected strikesReset to remain 1, got %d", h.strikesReset)
	}
}

func TestCleanup(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	now := time.Now()

	// Add strikes with different timestamps
	h.strikes["old1"] = &StrikeRecord{
		Count:     1,
		FirstSeen: now.Add(-48 * time.Hour),
		LastSeen:  now.Add(-48 * time.Hour),
		Job:       "job1",
	}
	h.strikes["old2"] = &StrikeRecord{
		Count:     2,
		FirstSeen: now.Add(-25 * time.Hour),
		LastSeen:  now.Add(-25 * time.Hour),
		Job:       "job2",
	}
	h.strikes["recent"] = &StrikeRecord{
		Count:     3,
		FirstSeen: now.Add(-1 * time.Hour),
		LastSeen:  now.Add(-1 * time.Hour),
		Job:       "job3",
	}

	// Clean up entries older than 24 hours
	removed := h.Cleanup(24 * time.Hour)

	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	if h.Count() != 1 {
		t.Errorf("expected 1 remaining record, got %d", h.Count())
	}

	if _, exists := h.strikes["recent"]; !exists {
		t.Error("expected recent record to remain")
	}

	if _, exists := h.strikes["old1"]; exists {
		t.Error("expected old1 to be removed")
	}

	if _, exists := h.strikes["old2"]; exists {
		t.Error("expected old2 to be removed")
	}
}

func TestSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "strikes.json")

	h1 := NewHandler(persistPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Add strikes
	h1.Add("dl1", "job1", "item1")
	h1.Add("dl1", "job1", "item1")
	h1.Add("dl2", "job2", "item2")

	// Save
	if err := h1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(persistPath); os.IsNotExist(err) {
		t.Fatal("persist file was not created")
	}

	// Load into new handler
	h2 := NewHandler(persistPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Verify counts match
	if h2.Get("dl1") != 2 {
		t.Errorf("expected dl1 count 2, got %d", h2.Get("dl1"))
	}
	if h2.Get("dl2") != 1 {
		t.Errorf("expected dl2 count 1, got %d", h2.Get("dl2"))
	}

	// Verify record details
	rec, exists := h2.GetRecord("dl1")
	if !exists {
		t.Fatal("expected dl1 record to exist")
	}
	if rec.Job != "job1" {
		t.Errorf("expected job 'job1', got '%s'", rec.Job)
	}
	if rec.Name != "item1" {
		t.Errorf("expected name 'item1', got '%s'", rec.Name)
	}
}

func TestSaveLoadEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "strikes.json")

	h1 := NewHandler(persistPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Save empty strikes
	if err := h1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load into new handler
	h2 := NewHandler(persistPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if h2.Count() != 0 {
		t.Errorf("expected 0 records, got %d", h2.Count())
	}
}

func TestSaveLoadCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "strikes.json")

	// Write corrupt JSON
	if err := os.WriteFile(persistPath, []byte("not valid json {]"), 0644); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}

	// Handler should load but start fresh on corrupt data
	h := NewHandler(persistPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if h.Count() != 0 {
		t.Errorf("expected 0 records after loading corrupt file, got %d", h.Count())
	}

	// Should still be able to add strikes
	h.Add("dl1", "job1", "item1")
	if h.Get("dl1") != 1 {
		t.Errorf("expected to be able to add strikes after corrupt load")
	}
}

func TestResetCycleCounters(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Add and reset strikes
	h.Add("dl1", "job1", "item1")
	h.Add("dl2", "job2", "item2")
	h.Add("dl3", "job3", "item3")
	h.Reset("dl1")
	h.Reset("dl2")

	// Get cycle counters
	added, reset := h.ResetCycleCounters()

	if added != 3 {
		t.Errorf("expected added 3, got %d", added)
	}
	if reset != 2 {
		t.Errorf("expected reset 2, got %d", reset)
	}

	// Verify counters are reset
	if h.strikesAdded != 0 {
		t.Errorf("expected strikesAdded 0 after reset, got %d", h.strikesAdded)
	}
	if h.strikesReset != 0 {
		t.Errorf("expected strikesReset 0 after reset, got %d", h.strikesReset)
	}

	// Add more strikes
	h.Add("dl4", "job4", "item4")
	added, reset = h.ResetCycleCounters()

	if added != 1 {
		t.Errorf("expected added 1 in new cycle, got %d", added)
	}
	if reset != 0 {
		t.Errorf("expected reset 0 in new cycle, got %d", reset)
	}
}

func TestConcurrentAccess(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	var wg sync.WaitGroup
	numGoroutines := 10
	operationsPerGoroutine := 100

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				h.Add("dl1", "job1", "item1")
			}
		}(i)
	}

	// Concurrent gets
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				_ = h.Get("dl1")
			}
		}()
	}

	// Concurrent resets and adds on different IDs
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			dlID := "dl" + string(rune('2'+id))
			for j := 0; j < operationsPerGoroutine/10; j++ {
				h.Add(dlID, "job", "item")
				h.Reset(dlID)
			}
		}(i)
	}

	wg.Wait()

	// Verify dl1 count is correct
	expectedCount := numGoroutines * operationsPerGoroutine
	if count := h.Get("dl1"); count != expectedCount {
		t.Errorf("expected count %d after concurrent adds, got %d", expectedCount, count)
	}
}

func TestGetRecord(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	now := time.Now()
	h.Add("dl1", "job1", "item1")

	// Get record
	rec, exists := h.GetRecord("dl1")
	if !exists {
		t.Fatal("expected record to exist")
	}

	if rec.Count != 1 {
		t.Errorf("expected count 1, got %d", rec.Count)
	}
	if rec.Job != "job1" {
		t.Errorf("expected job 'job1', got '%s'", rec.Job)
	}
	if rec.Name != "item1" {
		t.Errorf("expected name 'item1', got '%s'", rec.Name)
	}
	if rec.FirstSeen.Before(now.Add(-1 * time.Second)) {
		t.Error("FirstSeen is too old")
	}

	// Unknown ID
	_, exists = h.GetRecord("unknown")
	if exists {
		t.Error("expected record to not exist")
	}
}

func TestGetAllRecords(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	h.Add("dl1", "job1", "item1")
	h.Add("dl2", "job2", "item2")
	h.Add("dl2", "job2", "item2")

	records := h.GetAllRecords()

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if rec, ok := records["dl1"]; !ok || rec.Count != 1 {
		t.Error("dl1 record incorrect")
	}
	if rec, ok := records["dl2"]; !ok || rec.Count != 2 {
		t.Error("dl2 record incorrect")
	}

	// Verify it's a copy (mutation doesn't affect handler)
	records["dl1"].Count = 999
	if h.Get("dl1") != 1 {
		t.Error("GetAllRecords should return a copy")
	}
}

func TestClear(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	h.Add("dl1", "job1", "item1")
	h.Add("dl2", "job2", "item2")
	h.Add("dl3", "job3", "item3")

	if h.Count() != 3 {
		t.Fatalf("expected 3 records before clear, got %d", h.Count())
	}

	h.Clear()

	if h.Count() != 0 {
		t.Errorf("expected 0 records after clear, got %d", h.Count())
	}

	// Verify we can still add after clear
	h.Add("dl4", "job4", "item4")
	if h.Get("dl4") != 1 {
		t.Error("expected to be able to add after clear")
	}
}

func TestSaveNoPersistPath(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	h.Add("dl1", "job1", "item1")

	// Save should be no-op without error
	if err := h.Save(); err != nil {
		t.Errorf("Save without persist path should not error, got: %v", err)
	}
}

func TestLoadNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "nonexistent.json")

	// Load should succeed when file doesn't exist
	h := NewHandler(persistPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if h.Count() != 0 {
		t.Errorf("expected 0 records when file doesn't exist, got %d", h.Count())
	}
}

func TestSaveCreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "subdir", "nested", "strikes.json")

	h := NewHandler(persistPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	h.Add("dl1", "job1", "item1")

	// Save should create directory
	if err := h.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(persistPath); os.IsNotExist(err) {
		t.Error("persist file was not created in nested directory")
	}
}

func TestAddUpdatesMetadata(t *testing.T) {
	h := NewHandler("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// First add
	h.Add("dl1", "job1", "item1")
	rec1, _ := h.GetRecord("dl1")

	time.Sleep(10 * time.Millisecond)

	// Second add with different job and name
	h.Add("dl1", "job2", "item2")
	rec2, _ := h.GetRecord("dl1")

	if rec2.Count != 2 {
		t.Errorf("expected count 2, got %d", rec2.Count)
	}
	if rec2.Job != "job2" {
		t.Errorf("expected job to be updated to 'job2', got '%s'", rec2.Job)
	}
	if rec2.Name != "item2" {
		t.Errorf("expected name to be updated to 'item2', got '%s'", rec2.Name)
	}
	if !rec2.LastSeen.After(rec1.LastSeen) {
		t.Error("LastSeen should be updated on subsequent adds")
	}
	if !rec2.FirstSeen.Equal(rec1.FirstSeen) {
		t.Error("FirstSeen should not change on subsequent adds")
	}
}

func TestSaveAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "strikes.json")

	h := NewHandler(persistPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	h.Add("dl1", "job1", "item1")

	// Save
	if err := h.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify temp file was cleaned up
	tmpPath := persistPath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up after save")
	}

	// Verify content is valid JSON
	data, err := os.ReadFile(persistPath)
	if err != nil {
		t.Fatalf("failed to read persist file: %v", err)
	}

	var strikes map[string]*StrikeRecord
	if err := json.Unmarshal(data, &strikes); err != nil {
		t.Errorf("persist file contains invalid JSON: %v", err)
	}
}
