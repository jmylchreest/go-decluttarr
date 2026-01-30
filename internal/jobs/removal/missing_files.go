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

// MissingFilesJob removes downloads where files are missing on disk
type MissingFilesJob struct {
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

// NewMissingFilesJob creates a new missing files removal job
func NewMissingFilesJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *MissingFilesJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	return &MissingFilesJob{
		name:       name,
		enabled:    cfg.Enabled,
		cfg:        cfg,
		defaults:   defaults,
		manager:    manager,
		logger:     logger.With("job", "remove_missing_files"),
		testRun:    testRun,
		maxStrikes: maxStrikes,
	}
}

// Name returns the job name
func (j *MissingFilesJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *MissingFilesJob) Enabled() bool {
	return j.enabled
}

// FindAffected identifies items with missing files in the queue
func (j *MissingFilesJob) FindAffected(queue []arrapi.QueueItem) []arrapi.QueueItem {
	var affected []arrapi.QueueItem

	for _, item := range queue {
		if j.hasMissingFiles(&item) {
			affected = append(affected, item)
		}
	}

	return affected
}

// hasMissingFiles checks if a queue item has missing files based on status messages
func (j *MissingFilesJob) hasMissingFiles(item *arrapi.QueueItem) bool {
	// Check status messages for missing file indicators
	for _, msg := range item.StatusMessages {
		title := strings.ToLower(msg.Title)
		for _, message := range msg.Messages {
			messageLower := strings.ToLower(message)

			// Common indicators of missing files
			if strings.Contains(messageLower, "no files found") ||
				strings.Contains(messageLower, "missing files") ||
				strings.Contains(messageLower, "files are missing") ||
				strings.Contains(messageLower, "download folder doesn't contain") ||
				strings.Contains(title, "no files found") {
				return true
			}
		}
	}

	// Check error message
	if item.ErrorMessage != "" {
		errorLower := strings.ToLower(item.ErrorMessage)
		if strings.Contains(errorLower, "no files found") ||
			strings.Contains(errorLower, "missing files") {
			return true
		}
	}

	return false
}

// Run executes the missing files removal job
func (j *MissingFilesJob) Run(ctx context.Context) error {
	j.logger.Info("starting missing files removal job",
		"test_run", j.testRun,
		"max_strikes", j.maxStrikes)

	queues, err := j.manager.GetAllQueues(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queues: %w", err)
	}

	strikesHandler := j.manager.GetStrikesHandler()
	totalProcessed := 0
	totalRemoved := 0

	for instanceName, queue := range queues {
		affected := j.FindAffected(queue)
		j.logger.Info("found items with missing files",
			"instance", instanceName,
			"count", len(affected),
		)

		for _, item := range affected {
			totalProcessed++

			// Add strike for this download
			currentStrikes := strikesHandler.Add(item.DownloadID, j.name, item.Title)
			j.logger.Debug("added strike to item with missing files",
				"title", item.Title,
				"download_id", item.DownloadID,
				"strikes", currentStrikes,
				"max_strikes", j.maxStrikes,
				"instance", instanceName,
			)

			// Check if max strikes exceeded
			if strikesHandler.HasExceeded(item.DownloadID, j.maxStrikes) {
				if j.testRun {
					j.logger.Info("[TEST RUN] would remove item with missing files",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"instance", instanceName,
					)
				} else {
					if err := j.removeItem(ctx, instanceName, item); err != nil {
						j.logger.Error("failed to remove item with missing files",
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

					j.logger.Info("removed item with missing files",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"instance", instanceName,
					)
				}
			}
		}
	}

	j.logger.Info("missing files removal job completed",
		"processed", totalProcessed,
		"removed", totalRemoved,
		"test_run", j.testRun)

	j.lastFound = totalProcessed
	j.lastRemoved = totalRemoved

	return nil
}

// removeItem removes a queue item from the arr instance
func (j *MissingFilesJob) removeItem(ctx context.Context, instanceName string, item arrapi.QueueItem) error {
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
func (j *MissingFilesJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
