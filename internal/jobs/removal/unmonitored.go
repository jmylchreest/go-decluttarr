package removal

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/go-decluttarr/internal/arrapi"
	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/jobs"
)

// UnmonitoredJob removes downloads for unmonitored series/movies/albums/books
type UnmonitoredJob struct {
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

// NewUnmonitoredJob creates a new unmonitored removal job
func NewUnmonitoredJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *UnmonitoredJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	return &UnmonitoredJob{
		name:       name,
		enabled:    cfg.Enabled,
		cfg:        cfg,
		defaults:   defaults,
		manager:    manager,
		logger:     logger.With("job", "remove_unmonitored"),
		testRun:    testRun,
		maxStrikes: maxStrikes,
	}
}

// Name returns the job name
func (j *UnmonitoredJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *UnmonitoredJob) Enabled() bool {
	return j.enabled
}

// Run executes the unmonitored removal job
func (j *UnmonitoredJob) Run(ctx context.Context) error {
	j.logger.Debug("starting unmonitored removal job",
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
		client, ok := j.manager.GetArrClient(instanceName)
		if !ok {
			j.logger.Error("arr client not found", "instance", instanceName)
			continue
		}

		// Get system status to determine instance type
		systemStatus, err := client.GetSystemStatus(ctx)
		if err != nil {
			j.logger.Error("failed to get system status",
				"instance", instanceName,
				"error", err)
			continue
		}

		j.logger.Debug("detected arr instance type",
			"instance", instanceName,
			"app", systemStatus.AppName)

		for _, item := range queue {
			totalProcessed++

			isUnmonitored, err := j.checkUnmonitored(ctx, client, systemStatus.AppName, &item)
			if err != nil {
				j.logger.Error("failed to check monitored status",
					"instance", instanceName,
					"queue_id", item.ID,
					"error", err)
				continue
			}

			if !isUnmonitored {
				continue
			}

			j.logger.Debug("found unmonitored item",
				"instance", instanceName,
				"app", systemStatus.AppName,
				"queue_id", item.ID,
				"download_id", item.DownloadID,
				"title", item.Title)

			// Increment strikes
			currentStrikes := strikesHandler.Add(item.DownloadID, j.name, item.Title)
			j.logger.Debug("incremented strikes for unmonitored item",
				"download_id", item.DownloadID,
				"current_strikes", currentStrikes,
				"max_strikes", j.maxStrikes)

			// Check if strikes exceeded
			if !strikesHandler.HasExceeded(item.DownloadID, j.maxStrikes) {
				j.logger.Debug("unmonitored item has not exceeded max strikes yet",
					"download_id", item.DownloadID,
					"current_strikes", currentStrikes,
					"max_strikes", j.maxStrikes)
				continue
			}

			j.logger.Debug("unmonitored item exceeded max strikes",
				"instance", instanceName,
				"queue_id", item.ID,
				"download_id", item.DownloadID,
				"title", item.Title,
				"strikes", currentStrikes)

			// Determine removal action based on tracker type and protected tags
			action := j.manager.GetRemovalAction(ctx, item.DownloadID)

			switch action {
			case "skip":
				j.logger.Debug("skipping protected item", "title", item.Title, "download_id", item.DownloadID)
				continue
			case "tag":
				if j.testRun {
					j.logger.Info("[TEST RUN] would tag unmonitored item as obsolete",
						"instance", instanceName,
						"queue_id", item.ID,
						"download_id", item.DownloadID,
						"title", item.Title,
						"strikes", currentStrikes,
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
					j.logger.Info("tagged unmonitored item as obsolete",
						"instance", instanceName,
						"queue_id", item.ID,
						"download_id", item.DownloadID,
						"title", item.Title,
						"strikes", currentStrikes,
					)
				}
				strikesHandler.Reset(item.DownloadID)
				totalRemoved++ // Count as handled
				continue
			case "remove":
				// Proceed with removal
			}

			// Remove from queue if not in test run mode
			if !j.testRun {
				opts := arrapi.DeleteOptions{
					RemoveFromClient: true,
					Blocklist:        false,
					SkipRedownload:   true,
				}

				if err := client.DeleteQueueItem(ctx, item.ID, opts); err != nil {
					j.logger.Error("failed to remove queue item",
						"instance", instanceName,
						"queue_id", item.ID,
						"error", err)
					continue
				}

				j.logger.Info("removed unmonitored item",
					"instance", instanceName,
					"queue_id", item.ID,
					"download_id", item.DownloadID,
					"title", item.Title,
					"strikes", currentStrikes)

				// Reset strikes after successful removal
				strikesHandler.Reset(item.DownloadID)
				totalRemoved++
			} else {
				j.logger.Info("[TEST RUN] would remove unmonitored queue item",
					"instance", instanceName,
					"queue_id", item.ID,
					"download_id", item.DownloadID,
					"title", item.Title)
				totalRemoved++
			}
		}
	}

	j.logger.Debug("unmonitored removal job completed",
		"processed", totalProcessed,
		"removed", totalRemoved,
		"test_run", j.testRun)

	j.lastFound = totalProcessed
	j.lastRemoved = totalRemoved

	return nil
}

// checkUnmonitored determines if a queue item belongs to an unmonitored parent entity
func (j *UnmonitoredJob) checkUnmonitored(ctx context.Context, client *arrapi.Client, appName string, item *arrapi.QueueItem) (bool, error) {
	var entityType string
	var entityID *int

	switch appName {
	case "Sonarr":
		entityType = "series"
		entityID = item.SeriesID
	case "Radarr":
		entityType = "movie"
		entityID = item.MovieID
	case "Lidarr":
		entityType = "artist"
		entityID = item.ArtistID
	case "Readarr":
		entityType = "author"
		entityID = item.AuthorID
	default:
		j.logger.Warn("unknown arr application type",
			"app", appName)
		return false, nil
	}

	// If no entity ID is present, item cannot be checked
	if entityID == nil {
		return false, nil
	}

	// Get monitored status using the helper method
	monitored, err := client.GetMonitoredStatus(ctx, entityType, *entityID)
	if err != nil {
		return false, err
	}

	// Return true if unmonitored (monitored == false)
	return !monitored, nil
}

// Stats returns the statistics from the last job run
func (j *UnmonitoredJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
