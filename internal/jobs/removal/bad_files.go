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

// BadFilesJob removes downloads with quality issues (corrupt, wrong format, sample files)
type BadFilesJob struct {
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

// NewBadFilesJob creates a new bad files removal job
func NewBadFilesJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *BadFilesJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	return &BadFilesJob{
		name:       name,
		enabled:    cfg.Enabled,
		cfg:        cfg,
		defaults:   defaults,
		manager:    manager,
		logger:     logger.With("job", "remove_bad_files"),
		testRun:    testRun,
		maxStrikes: maxStrikes,
	}
}

// Name returns the job identifier
func (j *BadFilesJob) Name() string {
	return j.name
}

// Enabled returns whether this job is enabled
func (j *BadFilesJob) Enabled() bool {
	return j.enabled
}

// badFileKeywords contains keywords indicating bad/corrupt files
var badFileKeywords = []string{
	"sample",
	"corrupt",
	"wrong format",
	"invalid",
	"damaged",
	"incomplete",
	"verification failed",
	"crc mismatch",
	"checksum",
}

// FindAffected identifies bad file items in the queue
func (j *BadFilesJob) FindAffected(queue []arrapi.QueueItem) []arrapi.QueueItem {
	var affected []arrapi.QueueItem

	for _, item := range queue {
		if j.isBadFile(&item) {
			affected = append(affected, item)
		}
	}

	return affected
}

// isBadFile checks if a queue item has bad file indicators
func (j *BadFilesJob) isBadFile(item *arrapi.QueueItem) bool {
	// Check status messages for bad file indicators
	for _, statusMsg := range item.StatusMessages {
		titleLower := strings.ToLower(statusMsg.Title)
		for _, msg := range statusMsg.Messages {
			msgLower := strings.ToLower(msg)

			for _, keyword := range badFileKeywords {
				if strings.Contains(titleLower, keyword) || strings.Contains(msgLower, keyword) {
					return true
				}
			}
		}
	}

	// Also check error message
	if item.ErrorMessage != "" {
		errLower := strings.ToLower(item.ErrorMessage)
		for _, keyword := range badFileKeywords {
			if strings.Contains(errLower, keyword) {
				return true
			}
		}
	}

	return false
}

// Run executes the bad files removal job
func (j *BadFilesJob) Run(ctx context.Context) error {
	j.logger.Debug("starting bad files removal job",
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
		j.logger.Debug("checking queue items for bad files",
			"instance", instanceName,
			"count", len(queue))

		affected := j.FindAffected(queue)
		j.logger.Debug("found items with bad files",
			"instance", instanceName,
			"count", len(affected),
		)

		for _, item := range affected {
			totalProcessed++

			// Get reason for logging
			reason := j.getBadFileReason(&item)

			// Add strike for this download
			currentStrikes := strikesHandler.Add(item.DownloadID, j.name, item.Title)
			j.logger.Debug("added strike to bad file download",
				"title", item.Title,
				"download_id", item.DownloadID,
				"strikes", currentStrikes,
				"max_strikes", j.maxStrikes,
				"reason", reason,
				"instance", instanceName,
			)

			// Check if max strikes exceeded
			if strikesHandler.HasExceeded(item.DownloadID, j.maxStrikes) {
				if j.testRun {
					j.logger.Info("[TEST RUN] would remove bad file",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"reason", reason,
						"instance", instanceName,
					)
				} else {
					if err := j.removeItem(ctx, instanceName, item); err != nil {
						j.logger.Error("failed to remove bad file",
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

					j.logger.Info("removed bad file",
						"title", item.Title,
						"download_id", item.DownloadID,
						"reason", reason,
						"instance", instanceName,
					)
				}
			}
		}
	}

	j.logger.Debug("bad files removal job completed",
		"processed", totalProcessed,
		"removed", totalRemoved,
		"test_run", j.testRun)

	j.lastFound = totalProcessed
	j.lastRemoved = totalRemoved

	return nil
}

// getBadFileReason returns the reason why a file is considered bad
func (j *BadFilesJob) getBadFileReason(item *arrapi.QueueItem) string {
	for _, statusMsg := range item.StatusMessages {
		titleLower := strings.ToLower(statusMsg.Title)
		for _, msg := range statusMsg.Messages {
			msgLower := strings.ToLower(msg)

			for _, keyword := range badFileKeywords {
				if strings.Contains(titleLower, keyword) || strings.Contains(msgLower, keyword) {
					return fmt.Sprintf("%s: %s", statusMsg.Title, msg)
				}
			}
		}
	}

	if item.ErrorMessage != "" {
		errLower := strings.ToLower(item.ErrorMessage)
		for _, keyword := range badFileKeywords {
			if strings.Contains(errLower, keyword) {
				return item.ErrorMessage
			}
		}
	}

	return "unknown"
}

// removeItem removes a queue item from the arr instance
func (j *BadFilesJob) removeItem(ctx context.Context, instanceName string, item arrapi.QueueItem) error {
	client, ok := j.manager.GetArrClient(instanceName)
	if !ok {
		return fmt.Errorf("arr client not found: %s", instanceName)
	}

	opts := arrapi.DeleteOptions{
		RemoveFromClient: true,
		Blocklist:        true, // Blocklist bad files to prevent re-download
		SkipRedownload:   false,
	}

	return client.DeleteQueueItem(ctx, item.ID, opts)
}

// Stats returns the statistics from the last job run
func (j *BadFilesJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
