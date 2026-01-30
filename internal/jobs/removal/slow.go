package removal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmylchreest/go-declutarr/internal/arrapi"
	"github.com/jmylchreest/go-declutarr/internal/config"
	"github.com/jmylchreest/go-declutarr/internal/jobs"
)

// SlowDownloadJob removes downloads that are below the configured speed threshold
type SlowDownloadJob struct {
	name             string
	enabled          bool
	cfg              *config.JobConfig
	defaults         *config.JobDefaultsConfig
	manager          *jobs.Manager
	logger           *slog.Logger
	testRun          bool
	maxStrikes       int
	minDownloadSpeed float64
	lastFound        int
	lastRemoved      int
}

// NewSlowDownloadJob creates a new slow download removal job
func NewSlowDownloadJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *SlowDownloadJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	minDownloadSpeed := defaults.MinDownloadSpeed
	if cfg.MinDownloadSpeed != nil {
		minDownloadSpeed = *cfg.MinDownloadSpeed
	}

	return &SlowDownloadJob{
		name:             name,
		enabled:          cfg.Enabled,
		cfg:              cfg,
		defaults:         defaults,
		manager:          manager,
		logger:           logger.With("job", "remove_slow"),
		testRun:          testRun,
		maxStrikes:       maxStrikes,
		minDownloadSpeed: minDownloadSpeed,
	}
}

// Name returns the job identifier
func (j *SlowDownloadJob) Name() string {
	return j.name
}

// Enabled returns whether this job is enabled
func (j *SlowDownloadJob) Enabled() bool {
	return j.enabled
}

// FindAffected identifies slow download items in the queue
func (j *SlowDownloadJob) FindAffected(queue []arrapi.QueueItem) []arrapi.QueueItem {
	var affected []arrapi.QueueItem

	if j.minDownloadSpeed <= 0 {
		return affected
	}

	for _, item := range queue {
		// Skip if not downloading
		if item.Status != "downloading" {
			continue
		}

		// Calculate download speed (bytes per second)
		elapsed := time.Since(item.Added).Seconds()
		if elapsed < 60 { // Wait at least 1 minute before checking speed
			continue
		}

		downloaded := float64(item.Size - item.Sizeleft)
		speed := downloaded / elapsed // bytes per second

		if speed < j.minDownloadSpeed {
			affected = append(affected, item)
		}
	}

	return affected
}

// Run executes the slow download removal job
func (j *SlowDownloadJob) Run(ctx context.Context) error {
	j.logger.Info("starting slow download removal job",
		"test_run", j.testRun,
		"max_strikes", j.maxStrikes,
		"min_download_speed", j.minDownloadSpeed)

	if j.minDownloadSpeed <= 0 {
		j.logger.Debug("min download speed not configured, skipping")
		return nil
	}

	queues, err := j.manager.GetAllQueues(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queues: %w", err)
	}

	strikesHandler := j.manager.GetStrikesHandler()
	totalProcessed := 0
	totalRemoved := 0

	for instanceName, queue := range queues {
		j.logger.Debug("checking queue items for slow downloads",
			"instance", instanceName,
			"count", len(queue))

		for _, item := range queue {
			// Skip if not downloading
			if item.Status != "downloading" {
				continue
			}

			// Calculate download speed (bytes per second)
			elapsed := time.Since(item.Added).Seconds()
			if elapsed < 60 { // Wait at least 1 minute before checking speed
				j.logger.Debug("download too recent, skipping speed check",
					"title", item.Title,
					"elapsed_seconds", elapsed)
				continue
			}

			downloaded := float64(item.Size - item.Sizeleft)
			speed := downloaded / elapsed // bytes per second

			if speed < j.minDownloadSpeed {
				totalProcessed++
				j.logger.Debug("download is slow",
					"title", item.Title,
					"speed_bps", speed,
					"min_speed_bps", j.minDownloadSpeed)

				// Increment strikes
				currentStrikes := strikesHandler.Add(item.DownloadID, j.name, item.Title)
				j.logger.Info("added strike to slow download",
					"title", item.Title,
					"download_id", item.DownloadID,
					"strikes", currentStrikes,
					"max_strikes", j.maxStrikes,
					"speed_bps", speed,
					"instance", instanceName,
				)

				if strikesHandler.HasExceeded(item.DownloadID, j.maxStrikes) {
					if j.testRun {
						j.logger.Info("[TEST RUN] would remove slow download",
							"title", item.Title,
							"download_id", item.DownloadID,
							"speed_bps", speed,
							"instance", instanceName,
						)
					} else {
						if err := j.removeItem(ctx, instanceName, item); err != nil {
							j.logger.Error("failed to remove slow download",
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

						j.logger.Info("removed slow download",
							"title", item.Title,
							"download_id", item.DownloadID,
							"speed_bps", speed,
							"instance", instanceName,
						)
					}
				}
			} else {
				// Clear strikes if download speed is acceptable
				if strikesHandler.Get(item.DownloadID) > 0 {
					j.logger.Debug("download speed recovered, clearing strikes",
						"title", item.Title,
						"download_id", item.DownloadID)
					strikesHandler.Reset(item.DownloadID)
				}
			}
		}
	}

	j.logger.Info("slow download removal job completed",
		"processed", totalProcessed,
		"removed", totalRemoved,
		"test_run", j.testRun)

	j.lastFound = totalProcessed
	j.lastRemoved = totalRemoved

	return nil
}

// removeItem removes a queue item from the arr instance
func (j *SlowDownloadJob) removeItem(ctx context.Context, instanceName string, item arrapi.QueueItem) error {
	client, ok := j.manager.GetArrClient(instanceName)
	if !ok {
		return fmt.Errorf("arr client not found: %s", instanceName)
	}

	opts := arrapi.DeleteOptions{
		RemoveFromClient: true,
		Blocklist:        false,
		SkipRedownload:   false,
	}

	return client.DeleteQueueItem(ctx, item.ID, opts)
}

// Stats returns the statistics from the last job run
func (j *SlowDownloadJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
