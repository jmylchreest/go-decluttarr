package removal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jmylchreest/go-declutarr/internal/arrapi"
	"github.com/jmylchreest/go-declutarr/internal/config"
	"github.com/jmylchreest/go-declutarr/internal/jobs"
)

// FailedImportsJob removes failed import items from the queue
type FailedImportsJob struct {
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

// NewFailedImportsJob creates a new failed imports removal job
func NewFailedImportsJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *FailedImportsJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	return &FailedImportsJob{
		name:       name,
		enabled:    cfg.Enabled,
		cfg:        cfg,
		defaults:   defaults,
		manager:    manager,
		logger:     logger.With("job", "remove_failed_imports"),
		testRun:    testRun,
		maxStrikes: maxStrikes,
	}
}

// Name returns the job identifier
func (j *FailedImportsJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *FailedImportsJob) Enabled() bool {
	return j.enabled
}

// FindAffected identifies failed import items in the queue
func (j *FailedImportsJob) FindAffected(queue []arrapi.QueueItem) []arrapi.QueueItem {
	var affected []arrapi.QueueItem

	for _, item := range queue {
		if j.isFailedImport(item) {
			affected = append(affected, item)
		}
	}

	return affected
}

// isFailedImport determines if a queue item is a failed import
func (j *FailedImportsJob) isFailedImport(item arrapi.QueueItem) bool {
	// Primary indicator: TrackedDownloadState == "importFailed"
	// This means the download completed successfully but import failed
	if item.TrackedDownloadState == "importFailed" {
		return true
	}

	// Check for import-specific failures in status messages
	for _, msg := range item.StatusMessages {
		title := strings.ToLower(msg.Title)

		// Common import failure indicators
		if strings.Contains(title, "import") &&
		   (strings.Contains(title, "failed") ||
		    strings.Contains(title, "error") ||
		    strings.Contains(title, "unable")) {
			return true
		}

		// Specific import failure messages
		if msg.Title == "Import failed" ||
		   msg.Title == "No files found are eligible for import" ||
		   msg.Title == "Not a valid video file" ||
		   msg.Title == "Not an upgrade for existing file" ||
		   msg.Title == "Sample" {
			return true
		}
	}

	// Check error message for import failures
	if item.ErrorMessage != "" {
		errorLower := strings.ToLower(item.ErrorMessage)
		if strings.Contains(errorLower, "import") && strings.Contains(errorLower, "failed") {
			return true
		}
	}

	return false
}

// Run executes the failed imports removal job
func (j *FailedImportsJob) Run(ctx context.Context) error {
	j.logger.Info("starting failed imports removal job", "test_run", j.testRun, "max_strikes", j.maxStrikes)

	queues, err := j.manager.GetAllQueues(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queues: %w", err)
	}

	strikesHandler := j.manager.GetStrikesHandler()
	totalProcessed := 0
	totalRemoved := 0

	for instanceName, queue := range queues {
		affected := j.FindAffected(queue)
		j.logger.Info("found failed imports",
			"instance", instanceName,
			"count", len(affected),
		)

		for _, item := range affected {
			totalProcessed++

			// Add strike for this download
			currentStrikes := strikesHandler.Add(item.DownloadID, j.name, item.Title)
			j.logger.Debug("added strike to failed import",
				"title", item.Title,
				"download_id", item.DownloadID,
				"strikes", currentStrikes,
				"max_strikes", j.maxStrikes,
				"state", item.TrackedDownloadState,
				"status", item.TrackedDownloadStatus,
				"instance", instanceName,
			)

			// Check if max strikes exceeded
			if strikesHandler.HasExceeded(item.DownloadID, j.maxStrikes) {
				if j.testRun {
					j.logger.Info("[TEST RUN] would remove failed import",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"state", item.TrackedDownloadState,
						"status", item.TrackedDownloadStatus,
						"error", item.ErrorMessage,
						"instance", instanceName,
					)
				} else {
					if err := j.removeItem(ctx, instanceName, item); err != nil {
						j.logger.Error("failed to remove failed import",
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

					j.logger.Info("removed failed import",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"instance", instanceName,
					)
				}
			}
		}
	}

	j.logger.Info("failed imports removal job completed",
		"processed", totalProcessed,
		"removed", totalRemoved,
		"test_run", j.testRun,
	)

	j.lastFound = totalProcessed
	j.lastRemoved = totalRemoved

	return nil
}

// removeItem removes a queue item from the arr instance
func (j *FailedImportsJob) removeItem(ctx context.Context, instanceName string, item arrapi.QueueItem) error {
	client, ok := j.manager.GetArrClient(instanceName)
	if !ok {
		return fmt.Errorf("arr client not found: %s", instanceName)
	}

	opts := arrapi.DeleteOptions{
		RemoveFromClient: true,  // Remove from download client since download succeeded
		Blocklist:        false, // Don't blocklist - download was successful
		SkipRedownload:   true,  // Skip redownload since import failed (likely quality/format issue)
	}

	return client.DeleteQueueItem(ctx, item.ID, opts)
}

// Stats returns the statistics from the last job run
func (j *FailedImportsJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
