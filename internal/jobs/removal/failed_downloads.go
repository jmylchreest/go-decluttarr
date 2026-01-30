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

// FailedDownloadsJob removes failed downloads from the queue
type FailedDownloadsJob struct {
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

// NewFailedDownloadsJob creates a new failed downloads removal job
func NewFailedDownloadsJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *FailedDownloadsJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	return &FailedDownloadsJob{
		name:       name,
		enabled:    cfg.Enabled,
		cfg:        cfg,
		defaults:   defaults,
		manager:    manager,
		logger:     logger.With("job", "remove_failed_downloads"),
		testRun:    testRun,
		maxStrikes: maxStrikes,
	}
}

// Name returns the job identifier
func (j *FailedDownloadsJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *FailedDownloadsJob) Enabled() bool {
	return j.enabled
}

// FindAffected identifies failed download items in the queue
func (j *FailedDownloadsJob) FindAffected(queue []arrapi.QueueItem) []arrapi.QueueItem {
	var affected []arrapi.QueueItem

	for _, item := range queue {
		if j.isFailedDownload(item) {
			affected = append(affected, item)
		}
	}

	return affected
}

// isFailedDownload determines if a queue item is a failed download
func (j *FailedDownloadsJob) isFailedDownload(item arrapi.QueueItem) bool {
	// Check TrackedDownloadStatus for error/warning
	if item.TrackedDownloadStatus == "error" || item.TrackedDownloadStatus == "warning" {
		// Verify it's a download failure (not import failure)
		if item.TrackedDownloadState != "importFailed" {
			return true
		}
	}

	// Check status messages for download-specific failures
	for _, msg := range item.StatusMessages {
		title := strings.ToLower(msg.Title)

		// Common download failure indicators
		if strings.Contains(title, "download") &&
		   (strings.Contains(title, "failed") ||
		    strings.Contains(title, "error") ||
		    strings.Contains(title, "missing") ||
		    strings.Contains(title, "corrupt")) {
			return true
		}

		// Specific download failure messages
		if msg.Title == "Download client unavailable" ||
		   msg.Title == "No files found are eligible for import" ||
		   msg.Title == "Unable to determine if file is a sample" {
			return true
		}
	}

	// Check error message field
	if item.ErrorMessage != "" {
		errorLower := strings.ToLower(item.ErrorMessage)
		if strings.Contains(errorLower, "download") && strings.Contains(errorLower, "failed") {
			return true
		}
	}

	return false
}

// Run executes the failed downloads removal job
func (j *FailedDownloadsJob) Run(ctx context.Context) error {
	j.logger.Info("starting failed downloads removal job", "test_run", j.testRun, "max_strikes", j.maxStrikes)

	queues, err := j.manager.GetAllQueues(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queues: %w", err)
	}

	strikesHandler := j.manager.GetStrikesHandler()
	totalProcessed := 0
	totalRemoved := 0

	for instanceName, queue := range queues {
		affected := j.FindAffected(queue)
		j.logger.Info("found failed downloads",
			"instance", instanceName,
			"count", len(affected),
		)

		for _, item := range affected {
			totalProcessed++

			// Add strike for this download
			currentStrikes := strikesHandler.Add(item.DownloadID, j.name, item.Title)
			j.logger.Info("added strike to failed download",
				"title", item.Title,
				"download_id", item.DownloadID,
				"strikes", currentStrikes,
				"max_strikes", j.maxStrikes,
				"status", item.TrackedDownloadStatus,
				"error", item.ErrorMessage,
				"instance", instanceName,
			)

			// Check if max strikes exceeded
			if strikesHandler.HasExceeded(item.DownloadID, j.maxStrikes) {
				if j.testRun {
					j.logger.Info("[TEST RUN] would remove failed download",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"status", item.TrackedDownloadStatus,
						"error", item.ErrorMessage,
						"instance", instanceName,
					)
				} else {
					if err := j.removeItem(ctx, instanceName, item); err != nil {
						j.logger.Error("failed to remove failed download",
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

					j.logger.Info("removed failed download",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"instance", instanceName,
					)
				}
			}
		}
	}

	j.logger.Info("failed downloads removal job completed",
		"processed", totalProcessed,
		"removed", totalRemoved,
		"test_run", j.testRun,
	)

	j.lastFound = totalProcessed
	j.lastRemoved = totalRemoved

	return nil
}

// removeItem removes a queue item from the arr instance
func (j *FailedDownloadsJob) removeItem(ctx context.Context, instanceName string, item arrapi.QueueItem) error {
	client, ok := j.manager.GetArrClient(instanceName)
	if !ok {
		return fmt.Errorf("arr client not found: %s", instanceName)
	}

	opts := arrapi.DeleteOptions{
		RemoveFromClient: true,
		Blocklist:        true, // Blocklist failed downloads to prevent re-download
		SkipRedownload:   true,
	}

	return client.DeleteQueueItem(ctx, item.ID, opts)
}

// Stats returns the statistics from the last job run
func (j *FailedDownloadsJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
