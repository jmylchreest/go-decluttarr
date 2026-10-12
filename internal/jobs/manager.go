package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jmylchreest/go-decluttarr/internal/arrapi"
	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/downloadclient"
	"github.com/jmylchreest/go-decluttarr/internal/shelfarr"
	"github.com/jmylchreest/go-decluttarr/internal/strikes"
)

// CycleStats tracks statistics for a single execution cycle
type CycleStats struct {
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	JobsRun      int
	JobsFailed   int
	ItemsFound   map[string]int            // job name -> count found
	ItemsRemoved map[string]int            // job name -> count removed
	SkipReasons  map[string]map[string]int // job name -> fixed reason -> decision count
	StrikesAdded int
	StrikesReset int
	TotalStrikes int
	Shelfarr     map[string]*shelfarr.Snapshot
	Errors       []string
}

// Manager coordinates job execution across multiple *arr instances and download clients
type Manager struct {
	cfg             *config.Config
	logger          *slog.Logger
	jobs            []Job
	arrClients      map[string]*arrapi.Client // keyed by instance name
	downloadClients map[string]downloadclient.Client
	shelfarrClients map[string]*shelfarr.Client
	strikes         *strikes.Handler
	mu              sync.RWMutex
	runMu           sync.Mutex
	lastStats       *CycleStats
}

// NewManager creates a new job manager with the given configuration
func NewManager(cfg *config.Config, logger *slog.Logger, strikesPath string) *Manager {
	if logger == nil {
		logger = slog.Default()
	}

	return &Manager{
		cfg:             cfg,
		logger:          logger.With("component", "job_manager"),
		jobs:            make([]Job, 0),
		arrClients:      make(map[string]*arrapi.Client),
		downloadClients: make(map[string]downloadclient.Client),
		strikes:         strikes.NewHandler(strikesPath, logger),
		shelfarrClients: make(map[string]*shelfarr.Client),
	}
}

// RegisterJob adds a job to the manager's execution list
func (m *Manager) RegisterJob(job Job) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.jobs = append(m.jobs, job)
	m.logger.Debug("registered job", "job", job.Name())
}

// RunAll executes all enabled jobs - GRACEFUL: continues on error
func (m *Manager) RunAll(ctx context.Context) error {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	m.mu.RLock()
	jobs := slices.Clone(m.jobs)
	m.mu.RUnlock()

	// Initialize cycle stats
	stats := &CycleStats{
		StartTime:    time.Now(),
		ItemsFound:   make(map[string]int),
		ItemsRemoved: make(map[string]int),
		SkipReasons:  make(map[string]map[string]int),
		Errors:       make([]string, 0),
		Shelfarr:     make(map[string]*shelfarr.Snapshot),
	}

	var errs []error
	var failedJobs []string

	m.mu.RLock()
	shelfarrClients := make(map[string]*shelfarr.Client, len(m.shelfarrClients))
	for name, client := range m.shelfarrClients {
		shelfarrClients[name] = client
	}
	m.mu.RUnlock()
	for name, client := range shelfarrClients {
		snapshot, err := client.GetSnapshot(ctx)
		if err != nil {
			wrapped := fmt.Errorf("shelfarr %s: %w", name, err)
			errs = append(errs, wrapped)
			failedJobs = append(failedJobs, "shelfarr/"+name)
			stats.Errors = append(stats.Errors, wrapped.Error())
			m.logger.Warn("Shelfarr request monitoring failed; category protection remains active", "instance", name, "error", err)
			continue
		}
		stats.Shelfarr[name] = snapshot
		m.logger.Info("Shelfarr request snapshot", "instance", name, "observed", snapshot.Observed,
			"at_limit", snapshot.AtLimit, "status_counts", snapshot.StatusCounts, "attention_needed", snapshot.AttentionNeeded,
			"scope", "recent requests visible to token; not download ownership")
	}

	for _, job := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !job.Enabled() {
			m.logger.Debug("skipping disabled job", "job", job.Name())
			continue
		}

		m.logger.Debug("running job", "job", job.Name())
		stats.JobsRun++

		recorder := &skipRecorder{}
		jobCtx := context.WithValue(ctx, skipRecorderKey{}, recorder)
		if err := job.Run(jobCtx); err != nil {
			m.logger.Error("job failed, continuing", "job", job.Name(), "error", err)
			errs = append(errs, err)
			failedJobs = append(failedJobs, job.Name())
			stats.JobsFailed++
			stats.Errors = append(stats.Errors, fmt.Sprintf("%s: %v", job.Name(), err))
			// CONTINUE - don't terminate!
		} else {
			m.logger.Debug("job completed successfully", "job", job.Name())
		}

		stats.SkipReasons[job.Name()] = recorder.snapshot()

		// Collect job stats if available
		if sj, ok := job.(StatsJob); ok {
			jobStats := sj.Stats()
			stats.ItemsFound[job.Name()] = jobStats.Found
			stats.ItemsRemoved[job.Name()] = jobStats.Removed
		}
	}

	// Get strike stats and reset cycle counters
	stats.StrikesAdded, stats.StrikesReset = m.strikes.ResetCycleCounters()
	stats.TotalStrikes = m.strikes.Count()

	// Finalize timing
	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)

	// Remove expired state before persisting it.
	m.strikes.Cleanup(7 * 24 * time.Hour)

	// Save strikes to disk
	if err := m.strikes.Save(); err != nil {
		m.logger.Error("failed to save strikes", "error", err)
	}

	// Store stats for later access
	m.mu.Lock()
	m.lastStats = stats
	m.mu.Unlock()

	// Log cycle summary
	m.logCycleSummary(stats)

	if len(errs) > 0 {
		return fmt.Errorf("%d cycle operations failed: %v", len(errs), failedJobs)
	}

	return nil
}

// JobResult represents the result of a single job for structured logging
type JobResult struct {
	Found   int            `json:"found"`
	Removed int            `json:"removed"`
	Skipped map[string]int `json:"skipped,omitempty"`
}

// logCycleSummary outputs a summary of the execution cycle as structured log
func (m *Manager) logCycleSummary(stats *CycleStats) {
	// Calculate totals
	totalFound := 0
	totalRemoved := 0
	totalSkipped := 0
	for _, v := range stats.ItemsFound {
		totalFound += v
	}
	for _, v := range stats.ItemsRemoved {
		totalRemoved += v
	}

	for _, reasons := range stats.SkipReasons {
		for _, count := range reasons {
			totalSkipped += count
		}
	}

	// Build job results map with only non-zero entries
	jobResults := make(map[string]JobResult)
	for jobName, found := range stats.ItemsFound {
		removed := stats.ItemsRemoved[jobName]
		if found > 0 || removed > 0 {
			jobResults[jobName] = JobResult{Found: found, Removed: removed}
		}
	}

	for jobName, reasons := range stats.SkipReasons {
		if len(reasons) > 0 {
			result := jobResults[jobName]
			result.Skipped = reasons
			jobResults[jobName] = result
		}
	}

	// Single structured log entry for cycle summary
	m.logger.Info("cycle complete",
		slog.Group("cycle",
			slog.Duration("duration", stats.Duration.Round(time.Millisecond)),
			slog.Int("jobs_run", stats.JobsRun),
			slog.Int("jobs_failed", stats.JobsFailed),
		),
		slog.Group("totals",
			slog.Int("found", totalFound),
			slog.Int("removed", totalRemoved),
			slog.Int("skipped", totalSkipped),
		),
		slog.Group("strikes",
			slog.Int("added", stats.StrikesAdded),
			slog.Int("cleared", stats.StrikesReset),
			slog.Int("tracked", stats.TotalStrikes),
		),
		slog.Any("jobs", jobResults),
	)

	// Log errors separately if any
	if len(stats.Errors) > 0 {
		m.logger.Warn("cycle errors",
			slog.Int("count", len(stats.Errors)),
			slog.Any("errors", stats.Errors),
		)
	}
}

// GetLastStats returns the statistics from the last execution cycle
func (m *Manager) GetLastStats() *CycleStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastStats
}

// GetAllQueues fetches queue from all configured arr instances
func (m *Manager) GetAllQueues(ctx context.Context) (map[string][]arrapi.QueueItem, error) {
	clients := m.GetAllArrClients()

	result := make(map[string][]arrapi.QueueItem)
	var errs []error

	// Bound simultaneous requests while allowing independent instances to overlap.
	var wg sync.WaitGroup
	var resultMu sync.Mutex
	sem := make(chan struct{}, 4)
	for name, client := range clients {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return result, ctx.Err()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			queue, err := client.GetQueue(ctx)
			resultMu.Lock()
			defer resultMu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
				return
			}
			result[name] = queue
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		return result, fmt.Errorf("errors retrieving queues: %v", errs)
	}

	return result, nil
}

// RegisterArrClient adds an *arr client to the manager
func (m *Manager) RegisterArrClient(name string, client *arrapi.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.arrClients[name] = client
	m.logger.Debug("registered arr client", "instance", name)
}

// RegisterDownloadClient adds a download client to the manager
func (m *Manager) RegisterDownloadClient(name string, client downloadclient.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.downloadClients[name] = client
	m.logger.Debug("registered download client", "client", name)
}

// GetArrClient retrieves an *arr client by name
func (m *Manager) GetArrClient(name string) (*arrapi.Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.arrClients[name]
	return client, ok
}

// GetDownloadClient retrieves a download client by name
func (m *Manager) GetDownloadClient(name string) (downloadclient.Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.downloadClients[name]
	return client, ok
}

// GetStrikesHandler returns the strikes handler
func (m *Manager) GetStrikesHandler() *strikes.Handler {
	return m.strikes
}

// GetConfig returns the configuration
func (m *Manager) GetConfig() *config.Config {
	return m.cfg
}

// GetAllDownloadClients returns all registered download clients
func (m *Manager) GetAllDownloadClients() map[string]downloadclient.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	result := make(map[string]downloadclient.Client, len(m.downloadClients))
	for k, v := range m.downloadClients {
		if slices.Contains(m.cfg.General.IgnoreDownloadClients, k) {
			continue
		}
		result[k] = v
	}
	return result
}

// GetAllArrClients returns all registered arr clients
func (m *Manager) GetAllArrClients() map[string]*arrapi.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	result := make(map[string]*arrapi.Client, len(m.arrClients))
	for k, v := range m.arrClients {
		result[k] = v
	}
	return result
}

// GetRemovalAction determines what action to take for a download based on tracker type and protected tags.
// Returns: "remove", "tag", or "skip"
func (m *Manager) GetRemovalAction(ctx context.Context, downloadHash string) string {
	// Step 1: Check if protected tag exists on the torrent
	torrent, client := m.findTorrentByHash(ctx, downloadHash)
	if torrent == nil {
		m.logger.Debug("torrent not found for removal action check", "hash", downloadHash)
		return skipRemoval(ctx, "download_unavailable") // Ownership and protection could not be established.
	}

	return m.GetTorrentRemovalAction(ctx, torrent, client)
}

// GetTorrentRemovalAction evaluates a known download on its owning client.
// Direct-client jobs avoid a second lookup across unrelated clients.
func (m *Manager) GetTorrentRemovalAction(ctx context.Context, torrent *downloadclient.Torrent, client downloadclient.Client) string {
	if torrent == nil || client == nil {
		return skipRemoval(ctx, "download_unavailable")
	}
	downloadHash := torrent.Hash
	if m.IsShelfarrCategory(torrent.Category) {
		return skipRemoval(ctx, "shelfarr_category")
	}

	// Check for protected tag
	if m.cfg.General.ProtectedTag != "" {
		for _, tag := range torrent.Tags {
			if tag == m.cfg.General.ProtectedTag {
				m.logger.Debug("torrent has protected tag, skipping removal",
					"hash", downloadHash,
					"tag", m.cfg.General.ProtectedTag)
				return skipRemoval(ctx, "protected_tag")
			}
		}
	}

	// Step 2: Check tracker type (private vs public)
	isPrivate, err := client.IsPrivateTracker(ctx, downloadHash)
	if err != nil {
		m.logger.Warn("failed to determine tracker type, skipping removal",
			"hash", downloadHash,
			"error", err)
		return skipRemoval(ctx, "tracker_status_unavailable")
	}

	// Step 3: Apply configured handling based on tracker type
	var handling string
	if isPrivate {
		handling = m.cfg.General.PrivateTrackerHandling
		m.logger.Debug("applying private tracker handling",
			"hash", downloadHash,
			"handling", handling)
	} else {
		handling = m.cfg.General.PublicTrackerHandling
		m.logger.Debug("applying public tracker handling",
			"hash", downloadHash,
			"handling", handling)
	}

	// Map handling to action
	switch handling {
	case "remove":
		return "remove"
	case "skip", "keep":
		if isPrivate {
			return skipRemoval(ctx, "private_tracker_policy")
		}
		return skipRemoval(ctx, "public_tracker_policy")
	case "obsolete_tag":
		return "tag"
	default:
		m.logger.Warn("unknown tracker handling type, skipping removal",
			"handling", handling)
		return skipRemoval(ctx, "unknown_tracker_policy")
	}
}

// ApplyObsoleteTag adds the obsolete tag to a torrent
func (m *Manager) ApplyObsoleteTag(ctx context.Context, downloadHash string) error {
	if m.cfg.General.ObsoleteTag == "" {
		return fmt.Errorf("obsolete tag not configured")
	}

	torrent, client := m.findTorrentByHash(ctx, downloadHash)
	if torrent == nil {
		return fmt.Errorf("torrent not found: %s", downloadHash)
	}

	// Check if tag already exists
	for _, tag := range torrent.Tags {
		if tag == m.cfg.General.ObsoleteTag {
			m.logger.Debug("obsolete tag already exists", "hash", downloadHash)
			return nil
		}
	}

	// Add the tag
	if err := client.AddTags(ctx, downloadHash, []string{m.cfg.General.ObsoleteTag}); err != nil {
		return fmt.Errorf("failed to add obsolete tag: %w", err)
	}

	m.logger.Debug("added obsolete tag to torrent",
		"hash", downloadHash,
		"tag", m.cfg.General.ObsoleteTag)

	return nil
}

// findTorrentByHash finds a torrent across all download clients
func (m *Manager) findTorrentByHash(ctx context.Context, hash string) (*downloadclient.Torrent, downloadclient.Client) {
	clients := m.GetAllDownloadClients()

	for _, client := range clients {
		torrent, err := client.GetTorrent(ctx, hash)
		if err != nil {
			continue // Torrent not found in this client, try next
		}
		if torrent != nil {
			return torrent, client
		}
	}

	return nil, nil
}

// Close cleans up all resources
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Save strikes before closing
	if err := m.strikes.Save(); err != nil {
		m.logger.Error("failed to save strikes on close", "error", err)
	}

	// Close all arr clients
	for name, client := range m.arrClients {
		client.Close()
		m.logger.Debug("closed arr client", "instance", name)
	}

	for _, client := range m.shelfarrClients {
		client.Close()
	}
	m.logger.Info("job manager closed")
}

// IsIgnoredDownloadClient reports whether the operator excluded this named client.
func (m *Manager) IsIgnoredDownloadClient(name string) bool {
	return slices.Contains(m.cfg.General.IgnoreDownloadClients, name)
}

// RegisterShelfarrClient adds a read-only request monitor, not an arr queue owner.
func (m *Manager) RegisterShelfarrClient(name string, client *shelfarr.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shelfarrClients[name] = client
}

// IsShelfarrCategory excludes configured categories on every download client.
// This protection is independent of API availability and request-list visibility.
func (m *Manager) IsShelfarrCategory(category string) bool {
	for _, instance := range m.cfg.Instances.Shelfarr {
		if slices.Contains(instance.ProtectedCategories, category) {
			return true
		}
	}
	return false
}
