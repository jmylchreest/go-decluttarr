package removal

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/go-declutarr/internal/arrapi"
	"github.com/jmylchreest/go-declutarr/internal/config"
	"github.com/jmylchreest/go-declutarr/internal/jobs"
)

// StalledJob removes stalled downloads from the queue
type StalledJob struct {
	name        string
	enabled     bool
	cfg         *config.JobConfig
	defaults    *config.JobDefaultsConfig
	manager     *jobs.Manager
	logger      *slog.Logger
	testRun     bool
	maxStrikes  int
	lastFound   int
	lastRemoved int
}

// NewStalledJob creates a new stalled removal job
func NewStalledJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *StalledJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	return &StalledJob{
		name:       name,
		enabled:    cfg.Enabled,
		cfg:        cfg,
		defaults:   defaults,
		manager:    manager,
		logger:     logger.With("job", "remove_stalled"),
		testRun:    testRun,
		maxStrikes: maxStrikes,
	}
}

// Name returns the job identifier
func (j *StalledJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *StalledJob) Enabled() bool {
	return j.enabled
}

// FindAffected identifies stalled items in the queue
func (j *StalledJob) FindAffected(queue []arrapi.QueueItem) []arrapi.QueueItem {
	var affected []arrapi.QueueItem

	for _, item := range queue {
		if j.isStalledItem(item) {
			affected = append(affected, item)
		}
	}

	return affected
}

// isStalledItem determines if a queue item is stalled
func (j *StalledJob) isStalledItem(item arrapi.QueueItem) bool {
	// Check if TrackedDownloadState indicates stalled condition
	if item.TrackedDownloadState == "importPending" {
		// Item downloaded but stuck in import pending state
		return true
	}

	// Check if TrackedDownloadStatus indicates warning/stalled
	if item.TrackedDownloadStatus == "warning" {
		// Check status messages for stalled indicators
		for _, msg := range item.StatusMessages {
			// Common stalled indicators in status messages
			if msg.Title == "Download stalled" ||
				msg.Title == "No files found" ||
				msg.Title == "Sample" {
				return true
			}
		}
	}

	// Check if status field indicates stalled
	if item.Status == "warning" || item.Status == "stalled" {
		return true
	}

	return false
}

// Run executes the stalled removal job
func (j *StalledJob) Run(ctx context.Context) error {
	j.logger.Info("starting stalled removal job", "test_run", j.testRun, "max_strikes", j.maxStrikes)

	queues, err := j.manager.GetAllQueues(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queues: %w", err)
	}

	strikesHandler := j.manager.GetStrikesHandler()
	totalProcessed := 0
	totalRemoved := 0

	for instanceName, queue := range queues {
		affected := j.FindAffected(queue)
		j.logger.Info("found stalled items",
			"instance", instanceName,
			"count", len(affected),
		)

		for _, item := range affected {
			totalProcessed++

			// Add strike for this download
			currentStrikes := strikesHandler.Add(item.DownloadID, j.name, item.Title)
			j.logger.Debug("added strike to stalled download",
				"title", item.Title,
				"download_id", item.DownloadID,
				"strikes", currentStrikes,
				"max_strikes", j.maxStrikes,
				"instance", instanceName,
			)

			// Check if max strikes exceeded
			if strikesHandler.HasExceeded(item.DownloadID, j.maxStrikes) {
				if j.testRun {
					j.logger.Info("[TEST RUN] would remove stalled download",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"state", item.TrackedDownloadState,
						"status", item.TrackedDownloadStatus,
						"instance", instanceName,
					)
				} else {
					if err := j.removeItem(ctx, instanceName, item); err != nil {
						j.logger.Error("failed to remove stalled item",
							"title", item.Title,
							"download_id", item.DownloadID,
							"error", err,
							"instance", instanceName,
						)
						continue
					}

					// Reset strikes after successful removal
					strikesHandler.Reset(item.DownloadID)
					totalRemoved++

					j.logger.Info("removed stalled download",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"instance", instanceName,
					)
				}
			}
		}
	}

	j.logger.Info("stalled removal job completed",
		"processed", totalProcessed,
		"removed", totalRemoved,
		"test_run", j.testRun,
	)

	j.lastFound = totalProcessed
	j.lastRemoved = totalRemoved

	return nil
}

// removeItem removes a queue item from the arr instance
func (j *StalledJob) removeItem(ctx context.Context, instanceName string, item arrapi.QueueItem) error {
	client, ok := j.manager.GetArrClient(instanceName)
	if !ok {
		return fmt.Errorf("arr client not found: %s", instanceName)
	}

	opts := arrapi.DeleteOptions{
		RemoveFromClient: true,
		Blocklist:        false,
		SkipRedownload:   true,
	}

	return client.DeleteQueueItem(ctx, item.ID, opts)
}

// Stats returns the statistics from the last job run
func (j *StalledJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
