package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/go-declutarr/internal/arrapi"
	"github.com/jmylchreest/go-declutarr/internal/config"
	"github.com/jmylchreest/go-declutarr/internal/downloadclient"
	"github.com/jmylchreest/go-declutarr/internal/strikes"
)

// CycleStats tracks statistics for a single execution cycle
type CycleStats struct {
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	JobsRun      int
	JobsFailed   int
	ItemsFound   map[string]int // job name -> count found
	ItemsRemoved map[string]int // job name -> count removed
	StrikesAdded int
	StrikesReset int
	TotalStrikes int
	Errors       []string
}

// Manager coordinates job execution across multiple *arr instances and download clients
type Manager struct {
	cfg             *config.Config
	logger          *slog.Logger
	jobs            []Job
	arrClients      map[string]*arrapi.Client // keyed by instance name
	downloadClients map[string]downloadclient.Client
	strikes         *strikes.Handler
	mu              sync.RWMutex
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
	}
}

// RegisterJob adds a job to the manager's execution list
func (m *Manager) RegisterJob(job Job) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.jobs = append(m.jobs, job)
	m.logger.Info("registered job", "job", job.Name())
}

// RunAll executes all enabled jobs - GRACEFUL: continues on error
func (m *Manager) RunAll(ctx context.Context) error {
	m.mu.RLock()
	jobs := m.jobs
	m.mu.RUnlock()

	// Initialize cycle stats
	stats := &CycleStats{
		StartTime:    time.Now(),
		ItemsFound:   make(map[string]int),
		ItemsRemoved: make(map[string]int),
		Errors:       make([]string, 0),
	}

	var errs []error
	var failedJobs []string

	for _, job := range jobs {
		if !job.Enabled() {
			m.logger.Debug("skipping disabled job", "job", job.Name())
			continue
		}

		m.logger.Info("running job", "job", job.Name())
		stats.JobsRun++

		if err := job.Run(ctx); err != nil {
			m.logger.Error("job failed, continuing", "job", job.Name(), "error", err)
			errs = append(errs, err)
			failedJobs = append(failedJobs, job.Name())
			stats.JobsFailed++
			stats.Errors = append(stats.Errors, fmt.Sprintf("%s: %v", job.Name(), err))
			// CONTINUE - don't terminate!
		} else {
			m.logger.Info("job completed successfully", "job", job.Name())
		}

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

	// Save strikes to disk
	if err := m.strikes.Save(); err != nil {
		m.logger.Error("failed to save strikes", "error", err)
	}

	// Cleanup stale strikes (older than 7 days)
	m.strikes.Cleanup(7 * 24 * time.Hour)

	// Store stats for later access
	m.mu.Lock()
	m.lastStats = stats
	m.mu.Unlock()

	// Log cycle summary
	m.logCycleSummary(stats)

	if len(errs) > 0 {
		return fmt.Errorf("%d jobs failed: %v", len(errs), failedJobs)
	}

	return nil
}

// JobResult represents the result of a single job for structured logging
type JobResult struct {
	Found   int `json:"found"`
	Removed int `json:"removed"`
}

// logCycleSummary outputs a summary of the execution cycle as structured log
func (m *Manager) logCycleSummary(stats *CycleStats) {
	// Calculate totals
	totalFound := 0
	totalRemoved := 0
	for _, v := range stats.ItemsFound {
		totalFound += v
	}
	for _, v := range stats.ItemsRemoved {
		totalRemoved += v
	}

	// Build job results map with only non-zero entries
	jobResults := make(map[string]JobResult)
	for jobName, found := range stats.ItemsFound {
		removed := stats.ItemsRemoved[jobName]
		if found > 0 || removed > 0 {
			jobResults[jobName] = JobResult{Found: found, Removed: removed}
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
	m.mu.RLock()
	clients := m.arrClients
	m.mu.RUnlock()

	result := make(map[string][]arrapi.QueueItem)
	var errs []error

	for name, client := range clients {
		queue, err := client.GetQueue(ctx)
		if err != nil {
			m.logger.Error("failed to get queue", "instance", name, "error", err)
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}

		result[name] = queue
		m.logger.Debug("retrieved queue", "instance", name, "items", len(queue))
	}

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
	m.logger.Info("registered arr client", "instance", name)
}

// RegisterDownloadClient adds a download client to the manager
func (m *Manager) RegisterDownloadClient(name string, client downloadclient.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.downloadClients[name] = client
	m.logger.Info("registered download client", "client", name)
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

	m.logger.Info("job manager closed")
}
