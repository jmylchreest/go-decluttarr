package strikes

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StrikeRecord holds strike info with metadata
type StrikeRecord struct {
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Job       string    `json:"job"`
	Name      string    `json:"name,omitempty"`
}

// Handler manages strike tracking for download items with persistence
type Handler struct {
	strikes      map[string]*StrikeRecord // key: downloadID, value: strike record
	mu           sync.RWMutex
	persistPath  string
	logger       *slog.Logger
	strikesAdded int // count for current cycle
	strikesReset int // count for current cycle
}

// NewHandler creates a new strikes handler
func NewHandler(persistPath string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}

	h := &Handler{
		strikes:     make(map[string]*StrikeRecord),
		persistPath: persistPath,
		logger:      logger.With("component", "strikes"),
	}

	// Load persisted strikes if path provided
	if persistPath != "" {
		if err := h.Load(); err != nil {
			logger.Warn("failed to load persisted strikes, starting fresh", "error", err)
		}
	}

	return h
}

// Add increments the strike count for a download ID
func (h *Handler) Add(downloadID, job, name string) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	if record, exists := h.strikes[downloadID]; exists {
		record.Count++
		record.LastSeen = now
		record.Job = job
		if name != "" {
			record.Name = name
		}
	} else {
		h.strikes[downloadID] = &StrikeRecord{
			Count:     1,
			FirstSeen: now,
			LastSeen:  now,
			Job:       job,
			Name:      name,
		}
	}

	h.strikesAdded++
	return h.strikes[downloadID].Count
}

// Get returns the current strike count for a download ID
func (h *Handler) Get(downloadID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if record, exists := h.strikes[downloadID]; exists {
		return record.Count
	}
	return 0
}

// GetRecord returns the full strike record for a download ID
func (h *Handler) GetRecord(downloadID string) (*StrikeRecord, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	record, exists := h.strikes[downloadID]
	if !exists {
		return nil, false
	}
	// Return a copy
	copy := *record
	return &copy, true
}

// Reset clears the strike count for a download ID
func (h *Handler) Reset(downloadID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.strikes[downloadID]; exists {
		delete(h.strikes, downloadID)
		h.strikesReset++
	}
}

// Clear removes all strike records
func (h *Handler) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.strikes = make(map[string]*StrikeRecord)
}

// HasExceeded returns true if the download ID has exceeded the max strikes
func (h *Handler) HasExceeded(downloadID string, maxStrikes int) bool {
	return h.Get(downloadID) >= maxStrikes
}

// Count returns the total number of tracked items
func (h *Handler) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.strikes)
}

// GetAllRecords returns a copy of all strike records
func (h *Handler) GetAllRecords() map[string]*StrikeRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[string]*StrikeRecord, len(h.strikes))
	for k, v := range h.strikes {
		copy := *v
		result[k] = &copy
	}
	return result
}

// ResetCycleCounters resets the per-cycle counters and returns previous values
func (h *Handler) ResetCycleCounters() (added, reset int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	added = h.strikesAdded
	reset = h.strikesReset
	h.strikesAdded = 0
	h.strikesReset = 0
	return
}

// Save persists strikes to disk
func (h *Handler) Save() error {
	if h.persistPath == "" {
		return nil
	}

	h.mu.RLock()
	data, err := json.MarshalIndent(h.strikes, "", "  ")
	h.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("marshal strikes: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(h.persistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write atomically via temp file
	tmpPath := h.persistPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, h.persistPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	h.logger.Debug("persisted strikes", "path", h.persistPath, "count", len(h.strikes))
	return nil
}

// Load restores strikes from disk
func (h *Handler) Load() error {
	if h.persistPath == "" {
		return nil
	}

	data, err := os.ReadFile(h.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet, not an error
		}
		return fmt.Errorf("read file: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if err := json.Unmarshal(data, &h.strikes); err != nil {
		return fmt.Errorf("unmarshal strikes: %w", err)
	}

	h.logger.Debug("loaded persisted strikes", "path", h.persistPath, "count", len(h.strikes))
	return nil
}

// Cleanup removes stale strikes not seen in the given duration
func (h *Handler) Cleanup(maxAge time.Duration) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, record := range h.strikes {
		if record.LastSeen.Before(cutoff) {
			delete(h.strikes, id)
			removed++
		}
	}

	if removed > 0 {
		h.logger.Debug("cleaned up stale strikes", "removed", removed, "max_age", maxAge)
	}

	return removed
}
