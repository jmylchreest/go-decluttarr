package removal

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
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
		return j.matchesMessagePatterns(item)
	}

	// Check for import-specific failures in status messages
	for _, msg := range item.StatusMessages {
		title := strings.ToLower(msg.Title)

		// Common import failure indicators
		if strings.Contains(title, "import") &&
			(strings.Contains(title, "failed") ||
				strings.Contains(title, "error") ||
				strings.Contains(title, "unable")) {
			return j.matchesMessagePatterns(item)
		}

		// Specific import failure messages
		if msg.Title == "Import failed" ||
			msg.Title == "No files found are eligible for import" ||
			msg.Title == "Not a valid video file" ||
			msg.Title == "Not an upgrade for existing file" ||
			msg.Title == "Sample" {
			return j.matchesMessagePatterns(item)
		}
	}

	// Check error message for import failures
	if item.ErrorMessage != "" {
		errorLower := strings.ToLower(item.ErrorMessage)
		if strings.Contains(errorLower, "import") && strings.Contains(errorLower, "failed") {
			return j.matchesMessagePatterns(item)
		}
	}

	return false
}

// matchesMessagePatterns checks if the item's messages match configured patterns
// If no patterns are configured, returns true (matches everything)
// If patterns are configured, returns true only if at least one pattern matches
func (j *FailedImportsJob) matchesMessagePatterns(item arrapi.QueueItem) bool {
	// If no patterns configured, match everything (backward compatible)
	if len(j.cfg.MessagePatterns) == 0 {
		return true
	}

	// Check all status messages
	for _, statusMsg := range item.StatusMessages {
		if matchesPattern(statusMsg.Title, j.cfg.MessagePatterns) {
			return true
		}
		for _, msg := range statusMsg.Messages {
			if matchesPattern(msg, j.cfg.MessagePatterns) {
				return true
			}
		}
	}

	// Check error message
	if item.ErrorMessage != "" {
		if matchesPattern(item.ErrorMessage, j.cfg.MessagePatterns) {
			return true
		}
	}

	return false
}

// matchesPattern checks if a message matches any of the configured patterns
// Supports glob/wildcard matching using filepath.Match
func matchesPattern(message string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}

	messageLower := strings.ToLower(message)

	for _, pattern := range patterns {
		patternLower := strings.ToLower(pattern)

		// Try filepath.Match for standard glob patterns
		if matched, err := filepath.Match(patternLower, messageLower); err == nil && matched {
			return true
		}

		// Fallback to simple wildcard matching for patterns with *
		if strings.Contains(patternLower, "*") {
			if wildcardMatch(messageLower, patternLower) {
				return true
			}
		}

		// Exact match fallback
		if messageLower == patternLower {
			return true
		}
	}

	return false
}

// wildcardMatch performs simple wildcard matching
// Supports patterns like "*text*", "prefix*", "*suffix"
func wildcardMatch(text, pattern string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return text == pattern
	}

	// Check prefix
	if parts[0] != "" && !strings.HasPrefix(text, parts[0]) {
		return false
	}

	// Check suffix
	if parts[len(parts)-1] != "" && !strings.HasSuffix(text, parts[len(parts)-1]) {
		return false
	}

	// Check middle parts
	pos := len(parts[0])
	for i := 1; i < len(parts)-1; i++ {
		if parts[i] == "" {
			continue
		}
		idx := strings.Index(text[pos:], parts[i])
		if idx == -1 {
			return false
		}
		pos += idx + len(parts[i])
	}

	return true
}

// Run executes the failed imports removal job
func (j *FailedImportsJob) Run(ctx context.Context) error {
	j.logger.Debug("starting failed imports removal job", "test_run", j.testRun, "max_strikes", j.maxStrikes)

	queues, err := j.manager.GetAllQueues(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queues: %w", err)
	}

	strikesHandler := j.manager.GetStrikesHandler()
	totalProcessed := 0
	totalRemoved := 0

	for instanceName, queue := range queues {
		affected := j.FindAffected(queue)
		j.logger.Debug("found failed imports",
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
				// Determine removal action based on tracker type and protected tags
				action := j.manager.GetRemovalAction(ctx, item.DownloadID)

				switch action {
				case "skip":
					j.logger.Debug("skipping protected item", "title", item.Title, "download_id", item.DownloadID)
					continue
				case "tag":
					if j.testRun {
						j.logger.Info("[TEST RUN] would tag failed import as obsolete",
							"title", item.Title,
							"download_id", item.DownloadID,
							"strikes", currentStrikes,
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
						j.logger.Info("tagged failed import as obsolete",
							"title", item.Title,
							"download_id", item.DownloadID,
							"strikes", currentStrikes,
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

	j.logger.Debug("failed imports removal job completed",
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
