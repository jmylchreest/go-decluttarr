package removal

import (
	"context"
	"log/slog"

	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/jobs"
)

// OrphansJob removes orphaned downloads that aren't tracked by any *arr instance
type OrphansJob struct {
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

// NewOrphansJob creates a new orphans removal job
func NewOrphansJob(
	name string,
	cfg *config.JobConfig,
	defaults *config.JobDefaultsConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *OrphansJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}

	return &OrphansJob{
		name:       name,
		enabled:    cfg.Enabled,
		cfg:        cfg,
		defaults:   defaults,
		manager:    manager,
		logger:     logger.With("job", "remove_orphans"),
		testRun:    testRun,
		maxStrikes: maxStrikes,
	}
}

// Name returns the job name
func (j *OrphansJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *OrphansJob) Enabled() bool {
	return j.enabled
}

// Run executes the orphans removal job
func (j *OrphansJob) Run(ctx context.Context) error {
	j.logger.Debug("starting orphans removal job",
		"test_run", j.testRun,
		"max_strikes", j.maxStrikes)

	// Build a map of all download IDs tracked by *arr instances
	trackedDownloads := make(map[string]bool)

	queues, err := j.manager.GetAllQueues(ctx)
	if err != nil {
		j.logger.Warn("error getting some queues, continuing with available data", "error", err)
	}

	for instanceName, queue := range queues {
		for _, item := range queue {
			if item.DownloadID != "" {
				trackedDownloads[item.DownloadID] = true
			}
		}
		j.logger.Debug("retrieved queue from arr instance",
			"instance", instanceName,
			"tracked_items", len(queue))
	}

	j.logger.Debug("total tracked downloads across all arr instances",
		"count", len(trackedDownloads))

	// Get all download clients and their torrents
	downloadClients := j.manager.GetAllDownloadClients()
	if len(downloadClients) == 0 {
		j.logger.Warn("no download clients registered, skipping orphan check")
		return nil
	}

	strikesHandler := j.manager.GetStrikesHandler()
	orphanCount := 0
	removedCount := 0

	for clientName, client := range downloadClients {
		torrents, err := client.GetTorrents(ctx)
		if err != nil {
			j.logger.Error("failed to get torrents from client",
				"client", clientName,
				"error", err)
			continue
		}

		j.logger.Debug("retrieved torrents from download client",
			"client", clientName,
			"count", len(torrents))

		for _, torrent := range torrents {
			// Check if torrent is tracked by any *arr instance
			if trackedDownloads[torrent.Hash] {
				// Torrent is tracked, skip
				continue
			}

			orphanCount++
			j.logger.Debug("found orphaned torrent",
				"client", clientName,
				"hash", torrent.Hash,
				"name", torrent.Name,
				"state", torrent.State)

			// Increment strikes
			currentStrikes := strikesHandler.Add(torrent.Hash, j.name, torrent.Name)
			j.logger.Debug("incremented strikes for orphaned torrent",
				"hash", torrent.Hash,
				"current_strikes", currentStrikes,
				"max_strikes", j.maxStrikes)

			// Check if strikes exceeded
			if !strikesHandler.HasExceeded(torrent.Hash, j.maxStrikes) {
				j.logger.Debug("orphaned torrent has not exceeded max strikes yet",
					"hash", torrent.Hash,
					"current_strikes", currentStrikes,
					"max_strikes", j.maxStrikes)
				continue
			}

			j.logger.Debug("orphaned torrent exceeded max strikes",
				"hash", torrent.Hash,
				"name", torrent.Name,
				"strikes", currentStrikes)

			// Determine removal action based on tracker type and protected tags
			action := j.manager.GetRemovalAction(ctx, torrent.Hash)

			switch action {
			case "skip":
				j.logger.Debug("skipping protected orphaned torrent", "name", torrent.Name, "hash", torrent.Hash)
				continue
			case "tag":
				if j.testRun {
					j.logger.Info("[TEST RUN] would tag orphaned torrent as obsolete",
						"hash", torrent.Hash,
						"name", torrent.Name,
						"strikes", currentStrikes,
					)
				} else {
					if err := j.manager.ApplyObsoleteTag(ctx, torrent.Hash); err != nil {
						j.logger.Error("failed to tag orphaned torrent as obsolete",
							"hash", torrent.Hash,
							"error", err,
						)
						continue
					}
					j.logger.Info("tagged orphaned torrent as obsolete",
						"hash", torrent.Hash,
						"name", torrent.Name,
						"strikes", currentStrikes,
					)
				}
				strikesHandler.Reset(torrent.Hash)
				removedCount++ // Count as handled
				continue
			case "remove":
				// Proceed with removal
			}

			// Remove from download client if not in test run mode
			if !j.testRun {
				if err := client.DeleteTorrent(ctx, torrent.Hash, false); err != nil {
					j.logger.Error("failed to remove orphaned torrent",
						"hash", torrent.Hash,
						"error", err)
					continue
				}

				j.logger.Info("removed orphaned torrent",
					"hash", torrent.Hash,
					"name", torrent.Name,
					"strikes", currentStrikes)

				// Reset strikes after successful removal
				strikesHandler.Reset(torrent.Hash)
				removedCount++
			} else {
				j.logger.Info("[TEST RUN] would remove orphaned torrent",
					"hash", torrent.Hash,
					"name", torrent.Name)
				removedCount++
			}
		}
	}

	j.logger.Debug("orphans removal job completed",
		"orphans_found", orphanCount,
		"removed", removedCount,
		"test_run", j.testRun)

	j.lastFound = orphanCount
	j.lastRemoved = removedCount

	return nil
}

// Stats returns the statistics from the last job run
func (j *OrphansJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}
