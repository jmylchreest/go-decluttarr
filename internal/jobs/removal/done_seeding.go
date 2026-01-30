package removal

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmylchreest/go-declutarr/internal/config"
	"github.com/jmylchreest/go-declutarr/internal/downloadclient"
	"github.com/jmylchreest/go-declutarr/internal/jobs"
)

// DoneSeedingJob removes completed torrents that have met their seeding goals
type DoneSeedingJob struct {
	name         string
	enabled      bool
	cfg          *config.RemoveDoneSeedingConfig
	manager      *jobs.Manager
	logger       *slog.Logger
	testRun      bool
	lastFound    int
	lastRemoved  int
}

// NewDoneSeedingJob creates a new done seeding removal job
func NewDoneSeedingJob(
	name string,
	cfg *config.RemoveDoneSeedingConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *DoneSeedingJob {
	return &DoneSeedingJob{
		name:    name,
		enabled: cfg.Enabled,
		cfg:     cfg,
		manager: manager,
		logger:  logger.With("job", "remove_done_seeding"),
		testRun: testRun,
	}
}

// Name returns the job name
func (j *DoneSeedingJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *DoneSeedingJob) Enabled() bool {
	return j.enabled
}

// Run executes the done seeding removal job
func (j *DoneSeedingJob) Run(ctx context.Context) error {
	j.logger.Debug("starting done seeding removal job",
		"test_run", j.testRun,
		"target_tags", j.cfg.TargetTags,
		"target_categories", j.cfg.TargetCategories)

	// Get all download clients
	downloadClients := j.manager.GetAllDownloadClients()
	if len(downloadClients) == 0 {
		j.logger.Warn("no download clients registered, skipping done seeding check")
		return nil
	}

	foundCount := 0
	removedCount := 0

	for clientName, client := range downloadClients {
		// Only process qBittorrent clients (this feature is qBit-specific)
		if client.Name() != "qBittorrent" {
			j.logger.Debug("skipping non-qBittorrent client", "client", clientName)
			continue
		}

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
			// Check if torrent matches target categories or tags
			if !j.matchesTarget(&torrent) {
				continue
			}

			// Check if torrent is in a completed seeding state
			// We need to check against qBittorrent's raw states
			if !j.isCompletedState(&torrent) {
				continue
			}

			// Get torrent properties to check seeding limits
			props, err := client.GetTorrentProperties(ctx, torrent.Hash)
			if err != nil {
				j.logger.Warn("failed to get torrent properties, skipping",
					"hash", torrent.Hash,
					"name", torrent.Name,
					"error", err)
				continue
			}

			// Check if seeding goals are met
			if !j.seedingGoalsMet(&torrent, props) {
				continue
			}

			foundCount++
			j.logger.Debug("found torrent that completed seeding",
				"client", clientName,
				"hash", torrent.Hash,
				"name", torrent.Name,
				"ratio", torrent.Ratio,
				"ratio_limit", props.RatioLimit,
				"seed_time", torrent.SeedTime,
				"seed_time_limit", time.Duration(props.SeedingTimeLimit)*time.Second)

			// Remove from download client if not in test run mode
			if !j.testRun {
				if err := client.DeleteTorrent(ctx, torrent.Hash, false); err != nil {
					j.logger.Error("failed to remove torrent",
						"hash", torrent.Hash,
						"error", err)
					continue
				}

				j.logger.Info("removed torrent that completed seeding",
					"hash", torrent.Hash,
					"name", torrent.Name,
					"ratio", torrent.Ratio,
					"seed_time", torrent.SeedTime)

				removedCount++
			} else {
				j.logger.Info("[TEST RUN] would remove torrent that completed seeding",
					"hash", torrent.Hash,
					"name", torrent.Name,
					"ratio", torrent.Ratio,
					"seed_time", torrent.SeedTime)
				removedCount++
			}
		}
	}

	j.logger.Debug("done seeding removal job completed",
		"found", foundCount,
		"removed", removedCount,
		"test_run", j.testRun)

	j.lastFound = foundCount
	j.lastRemoved = removedCount

	return nil
}

// Stats returns the statistics from the last job run
func (j *DoneSeedingJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastRemoved,
	}
}

// matchesTarget checks if torrent matches target categories or tags
func (j *DoneSeedingJob) matchesTarget(torrent *downloadclient.Torrent) bool {
	// Check if category matches
	if len(j.cfg.TargetCategories) > 0 {
		for _, category := range j.cfg.TargetCategories {
			if torrent.Category == category {
				return true
			}
		}
	}

	// Check if any tag matches
	if len(j.cfg.TargetTags) > 0 {
		for _, targetTag := range j.cfg.TargetTags {
			for _, torrentTag := range torrent.Tags {
				if torrentTag == targetTag {
					return true
				}
			}
		}
	}

	// If no target categories or tags configured, match all
	if len(j.cfg.TargetCategories) == 0 && len(j.cfg.TargetTags) == 0 {
		return true
	}

	return false
}

// isCompletedState checks if the torrent is in a completed seeding state
// For qBittorrent, completed states are "stoppedUP" and "pausedUP"
// Since we only have the mapped state, we check if it's paused/seeding
func (j *DoneSeedingJob) isCompletedState(torrent *downloadclient.Torrent) bool {
	// A torrent is considered "done" if it's fully downloaded (progress = 1.0)
	// and in a paused or seeding state
	if torrent.Progress < 1.0 {
		return false
	}

	// Check if in paused or seeding state (mapped from stoppedUP/pausedUP)
	return torrent.State == downloadclient.StatePaused || torrent.State == downloadclient.StateSeeding
}

// seedingGoalsMet checks if the torrent has met its seeding goals
func (j *DoneSeedingJob) seedingGoalsMet(torrent *downloadclient.Torrent, props *downloadclient.TorrentProperties) bool {
	// At least one seeding goal must be met
	ratioMet := false
	timeMet := false

	// Check ratio limit (must be > 0 to be active)
	if props.RatioLimit > 0 {
		if torrent.Ratio >= props.RatioLimit {
			ratioMet = true
			j.logger.Debug("ratio limit met",
				"hash", torrent.Hash,
				"ratio", torrent.Ratio,
				"limit", props.RatioLimit)
		}
	}

	// Check seeding time limit (must be > 0 to be active)
	if props.SeedingTimeLimit > 0 {
		seedTimeLimitDuration := time.Duration(props.SeedingTimeLimit) * time.Second
		if torrent.SeedTime >= seedTimeLimitDuration {
			timeMet = true
			j.logger.Debug("seeding time limit met",
				"hash", torrent.Hash,
				"seed_time", torrent.SeedTime,
				"limit", seedTimeLimitDuration)
		}
	}

	// Return true if at least one goal is met
	return ratioMet || timeMet
}
