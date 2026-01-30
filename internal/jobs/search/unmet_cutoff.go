package search

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmylchreest/go-declutarr/internal/arrapi"
	"github.com/jmylchreest/go-declutarr/internal/config"
	"github.com/jmylchreest/go-declutarr/internal/jobs"
)

// UnmetCutoffJob searches for items that don't meet quality cutoff
type UnmetCutoffJob struct {
	name                   string
	enabled                bool
	cfg                    *config.SearchJobConfig
	manager                *jobs.Manager
	logger                 *slog.Logger
	testRun                bool
	minDaysBetweenSearches int
	maxConcurrentSearches  int
	lastFound              int
	lastSearched           int
}

// NewUnmetCutoffJob creates a new cutoff search job
func NewUnmetCutoffJob(
	name string,
	cfg *config.SearchJobConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *UnmetCutoffJob {
	return &UnmetCutoffJob{
		name:                   name,
		enabled:                cfg.Enabled,
		cfg:                    cfg,
		manager:                manager,
		logger:                 logger.With("job", name),
		testRun:                testRun,
		minDaysBetweenSearches: cfg.MinDaysBetweenSearches,
		maxConcurrentSearches:  cfg.MaxConcurrentSearches,
	}
}

// Name returns the job identifier
func (j *UnmetCutoffJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *UnmetCutoffJob) Enabled() bool {
	return j.enabled
}

// Stats returns the statistics from the last run
func (j *UnmetCutoffJob) Stats() jobs.JobStats {
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastSearched,
	}
}

// Run executes the unmet cutoff search job
func (j *UnmetCutoffJob) Run(ctx context.Context) error {
	j.logger.Info("starting unmet cutoff search job",
		"test_run", j.testRun,
		"min_days_between_searches", j.minDaysBetweenSearches,
		"max_concurrent_searches", j.maxConcurrentSearches,
	)

	// Reset stats for this run
	j.lastFound = 0
	j.lastSearched = 0

	// Get all arr clients from manager
	allClients := j.getAllArrClients()
	if len(allClients) == 0 {
		j.logger.Warn("no arr clients registered")
		return nil
	}

	// Process each arr instance
	for instanceName, client := range allClients {
		if err := j.processArrInstance(ctx, instanceName, client); err != nil {
			j.logger.Error("failed to process arr instance",
				"instance", instanceName,
				"error", err)
			// Continue processing other instances
			continue
		}
	}

	j.logger.Info("unmet cutoff search job completed",
		"found", j.lastFound,
		"searched", j.lastSearched)

	return nil
}

// getAllArrClients retrieves all registered arr clients from the manager
func (j *UnmetCutoffJob) getAllArrClients() map[string]*arrapi.Client {
	return j.manager.GetAllArrClients()
}

// processArrInstance processes a single arr instance
func (j *UnmetCutoffJob) processArrInstance(ctx context.Context, instanceName string, client *arrapi.Client) error {
	// Get system status to determine arr type
	status, err := client.GetSystemStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system status: %w", err)
	}

	j.logger.Info("processing arr instance",
		"instance", instanceName,
		"type", status.AppName)

	switch status.AppName {
	case "Sonarr":
		return j.processSonarr(ctx, instanceName, client)
	case "Radarr":
		return j.processRadarr(ctx, instanceName, client)
	default:
		j.logger.Warn("unsupported arr type for cutoff search",
			"instance", instanceName,
			"type", status.AppName)
		return nil
	}
}

// processSonarr processes cutoff unmet items for a Sonarr instance
func (j *UnmetCutoffJob) processSonarr(ctx context.Context, instanceName string, client *arrapi.Client) error {
	sonarrClient := &arrapi.SonarrClient{Client: client}

	// Get cutoff unmet episodes
	items, err := sonarrClient.GetCutoffUnmet(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cutoff unmet episodes: %w", err)
	}

	j.logger.Info("found cutoff unmet episodes",
		"instance", instanceName,
		"count", len(items))
	j.lastFound += len(items)

	// Group episodes by series for efficient searching
	episodesBySeriesAndSeason := make(map[int]map[int][]int)
	for _, item := range items {
		if !item.Monitored {
			j.logger.Debug("skipping unmonitored episode",
				"instance", instanceName,
				"title", item.Title)
			continue
		}

		if item.SeriesID == nil || item.SeasonNumber == nil || item.ID == 0 {
			j.logger.Warn("invalid episode data",
				"instance", instanceName,
				"title", item.Title)
			continue
		}

		seriesID := *item.SeriesID
		seasonNum := *item.SeasonNumber

		if episodesBySeriesAndSeason[seriesID] == nil {
			episodesBySeriesAndSeason[seriesID] = make(map[int][]int)
		}
		episodesBySeriesAndSeason[seriesID][seasonNum] = append(episodesBySeriesAndSeason[seriesID][seasonNum], item.ID)
	}

	// Trigger searches
	searchCount := 0
	for seriesID, seasonMap := range episodesBySeriesAndSeason {
		for seasonNum, episodeIDs := range seasonMap {
			// Check if we've reached max concurrent searches
			if j.maxConcurrentSearches > 0 && searchCount >= j.maxConcurrentSearches {
				j.logger.Info("reached max concurrent searches limit",
					"instance", instanceName,
					"limit", j.maxConcurrentSearches)
				break
			}

			if j.testRun {
				j.logger.Debug("TEST RUN: would search episodes",
					"instance", instanceName,
					"series_id", seriesID,
					"season", seasonNum,
					"episode_count", len(episodeIDs))
			} else {
				j.logger.Debug("searching episodes",
					"instance", instanceName,
					"series_id", seriesID,
					"season", seasonNum,
					"episode_count", len(episodeIDs))

				if err := sonarrClient.SearchEpisodes(ctx, episodeIDs); err != nil {
					j.logger.Error("failed to search episodes",
						"instance", instanceName,
						"series_id", seriesID,
						"season", seasonNum,
						"error", err)
					continue
				}

				// Add delay between searches to avoid overwhelming the arr instance
				time.Sleep(2 * time.Second)
			}

			searchCount++
			j.lastSearched += len(episodeIDs)
		}
	}

	return nil
}

// processRadarr processes cutoff unmet items for a Radarr instance
func (j *UnmetCutoffJob) processRadarr(ctx context.Context, instanceName string, client *arrapi.Client) error {
	radarrClient := &arrapi.RadarrClient{Client: client}

	// Get cutoff unmet movies
	items, err := radarrClient.GetCutoffUnmet(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cutoff unmet movies: %w", err)
	}

	j.logger.Info("found cutoff unmet movies",
		"instance", instanceName,
		"count", len(items))
	j.lastFound += len(items)

	// Search for each movie
	searchCount := 0
	for _, item := range items {
		if !item.Monitored {
			j.logger.Debug("skipping unmonitored movie",
				"instance", instanceName,
				"title", item.Title)
			continue
		}

		if item.MovieID == nil {
			j.logger.Warn("invalid movie data",
				"instance", instanceName,
				"title", item.Title)
			continue
		}

		// Check if we've reached max concurrent searches
		if j.maxConcurrentSearches > 0 && searchCount >= j.maxConcurrentSearches {
			j.logger.Info("reached max concurrent searches limit",
				"instance", instanceName,
				"limit", j.maxConcurrentSearches)
			break
		}

		movieID := *item.MovieID

		if j.testRun {
			j.logger.Debug("TEST RUN: would search movie",
				"instance", instanceName,
				"movie_id", movieID,
				"title", item.Title)
		} else {
			j.logger.Debug("searching movie",
				"instance", instanceName,
				"movie_id", movieID,
				"title", item.Title)

			if err := radarrClient.SearchMovie(ctx, movieID); err != nil {
				j.logger.Error("failed to search movie",
					"instance", instanceName,
					"movie_id", movieID,
					"error", err)
				continue
			}

			// Add delay between searches to avoid overwhelming the arr instance
			time.Sleep(2 * time.Second)
		}

		searchCount++
		j.lastSearched++
	}

	return nil
}
