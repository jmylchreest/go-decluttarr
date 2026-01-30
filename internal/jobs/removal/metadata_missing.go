package removal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jmylchreest/go-decluttarr/internal/arrapi"
	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/jobs"
)

// MetadataMissingJob removes downloads that cannot be matched to library items
type MetadataMissingJob struct {
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

// NewMetadataMissingJob creates a new metadata missing removal job
func NewMetadataMissingJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *MetadataMissingJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	return &MetadataMissingJob{
		name:       name,
		enabled:    cfg.Enabled,
		cfg:        cfg,
		defaults:   defaults,
		manager:    manager,
		logger:     logger.With("job", "remove_metadata_missing"),
		testRun:    testRun,
		maxStrikes: maxStrikes,
	}
}

// Name returns the job identifier
func (j *MetadataMissingJob) Name() string {
	return j.name
}

// Enabled returns whether this job is enabled
func (j *MetadataMissingJob) Enabled() bool {
	return j.enabled
}

// metadataKeywords contains keywords indicating metadata/parsing issues
var metadataKeywords = []string{
	"unable to parse",
	"unknown series",
	"unknown movie",
	"unknown artist",
	"unknown author",
	"not found in library",
	"no match found",
	"parsing failed",
	"cannot identify",
	"metadata error",
	"series not found",
	"movie not found",
}

// FindAffected identifies items with metadata issues in the queue
func (j *MetadataMissingJob) FindAffected(queue []arrapi.QueueItem) []arrapi.QueueItem {
	var affected []arrapi.QueueItem

	for _, item := range queue {
		if j.hasMetadataIssue(&item) {
			affected = append(affected, item)
		}
	}

	return affected
}

// hasMetadataIssue checks if a queue item has metadata issues
func (j *MetadataMissingJob) hasMetadataIssue(item *arrapi.QueueItem) bool {
	// Check if item has library ID based on type
	hasLibraryMatch := false
	switch {
	case item.SeriesID != nil && *item.SeriesID > 0:
		hasLibraryMatch = true
	case item.MovieID != nil && *item.MovieID > 0:
		hasLibraryMatch = true
	case item.ArtistID != nil && *item.ArtistID > 0:
		hasLibraryMatch = true
	case item.AuthorID != nil && *item.AuthorID > 0:
		hasLibraryMatch = true
	}

	// If it has a library match, it's not a metadata issue
	if hasLibraryMatch {
		return false
	}

	// Check TrackedDownloadStatus for metadata issues
	if item.TrackedDownloadStatus != "warning" && item.TrackedDownloadStatus != "error" {
		return false
	}

	// Check status messages for metadata keywords
	for _, statusMsg := range item.StatusMessages {
		titleLower := strings.ToLower(statusMsg.Title)
		for _, msg := range statusMsg.Messages {
			msgLower := strings.ToLower(msg)

			for _, keyword := range metadataKeywords {
				if strings.Contains(titleLower, keyword) || strings.Contains(msgLower, keyword) {
					return true
				}
			}
		}
	}

	// Also check error message
	if item.ErrorMessage != "" {
		errLower := strings.ToLower(item.ErrorMessage)
		for _, keyword := range metadataKeywords {
			if strings.Contains(errLower, keyword) {
				return true
			}
		}
	}

	return false
}

// Run executes the metadata missing removal job
func (j *MetadataMissingJob) Run(ctx context.Context) error {
	j.logger.Debug("starting metadata missing removal job",
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
		j.logger.Debug("checking queue items for metadata issues",
			"instance", instanceName,
			"count", len(queue))

		affected := j.FindAffected(queue)
		j.logger.Debug("found items with metadata issues",
			"instance", instanceName,
			"count", len(affected),
		)

		for _, item := range affected {
			totalProcessed++

			// Get reason for logging
			reason := j.getMetadataIssueReason(&item)

			// Add strike for this download
			currentStrikes := strikesHandler.Add(item.DownloadID, j.name, item.Title)
			j.logger.Debug("added strike to metadata-failed download",
				"title", item.Title,
				"download_id", item.DownloadID,
				"strikes", currentStrikes,
				"max_strikes", j.maxStrikes,
				"reason", reason,
				"instance", instanceName,
			)

			// Check if max strikes exceeded
			if strikesHandler.HasExceeded(item.DownloadID, j.maxStrikes) {
				// Determine removal action based on tracker type and protected tags
				action := j.manager.GetRemovalAction(ctx, item.DownloadID)

				switch action {
				case "skip":
					j.logger.Debug("skipping protected item", "title", item.Title, "download_id", item.DownloadID)
					continue
				case "tag":
					if j.testRun {
						j.logger.Info("[TEST RUN] would tag metadata-failed download as obsolete",
							"title", item.Title,
							"download_id", item.DownloadID,
							"strikes", currentStrikes,
							"reason", reason,
							"instance", instanceName,
						)
					} else {
						if err := j.manager.ApplyObsoleteTag(ctx, item.DownloadID); err != nil {
							j.logger.Error("failed to tag as obsolete",
								"title", item.Title,
								"download_id", item.DownloadID,
								"error", err,
							)
							continue
						}
						j.logger.Info("tagged metadata-failed download as obsolete",
							"title", item.Title,
							"download_id", item.DownloadID,
							"strikes", currentStrikes,
							"reason", reason,
							"instance", instanceName,
						)
					}
					strikesHandler.Reset(item.DownloadID)
					totalRemoved++ // Count as handled
					continue
				case "remove":
					// Proceed with removal
				}

				if j.testRun {
					j.logger.Info("[TEST RUN] would remove metadata-failed download",
						"title", item.Title,
						"download_id", item.DownloadID,
						"strikes", currentStrikes,
						"reason", reason,
						"instance", instanceName,
					)
				} else {
					if err := j.removeItem(ctx, instanceName, item); err != nil {
						j.logger.Error("failed to remove metadata-failed download",
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

					j.logger.Info("removed metadata-failed download",
						"title", item.Title,
						"download_id", item.DownloadID,
						"reason", reason,
						"instance", instanceName,
					)
				}
			}
		}
	}

	j.logger.Debug("metadata missing removal job completed",
		"processed", totalProcessed,
		"removed", totalRemoved,
		"test_run", j.testRun)

	j.lastFound = totalProcessed
	j.lastRemoved = totalRemoved

	return nil
}

// getMetadataIssueReason returns the reason for the metadata issue
func (j *MetadataMissingJob) getMetadataIssueReason(item *arrapi.QueueItem) string {
	for _, statusMsg := range item.StatusMessages {
		titleLower := strings.ToLower(statusMsg.Title)
		for _, msg := range statusMsg.Messages {
			msgLower := strings.ToLower(msg)

			for _, keyword := range metadataKeywords {
				if strings.Contains(titleLower, keyword) || strings.Contains(msgLower, keyword) {
					return fmt.Sprintf("%s: %s", statusMsg.Title, msg)
				}
			}
		}
	}

	if item.ErrorMessage != "" {
		errLower := strings.ToLower(item.ErrorMessage)
		for _, keyword := range metadataKeywords {
			if strings.Contains(errLower, keyword) {
				return item.ErrorMessage
			}
		}
	}

	return "unknown"
}

// removeItem removes a queue item from the arr instance
func (j *MetadataMissingJob) removeItem(ctx context.Context, instanceName string, item arrapi.QueueItem) error {
	client, ok := j.manager.GetArrClient(instanceName)
	if !ok {
		return fmt.Errorf("arr client not found: %s", instanceName)
	}

	opts := arrapi.DeleteOptions{
		RemoveFromClient: true,
		Blocklist:        false, // Don't blocklist, might be parseable later
		SkipRedownload:   true,  // Skip redownload since we can't match it
	}

	return client.DeleteQueueItem(ctx, item.ID, opts)
}

// Stats returns the statistics from the last job run
func (j *MetadataMissingJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
