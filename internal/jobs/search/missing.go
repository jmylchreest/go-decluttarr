package search

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/go-declutarr/internal/arrapi"
	"github.com/jmylchreest/go-declutarr/internal/config"
	"github.com/jmylchreest/go-declutarr/internal/jobs"
)

// MissingJob searches for missing episodes/movies
type MissingJob struct {
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
	mu                     sync.RWMutex
}

// NewMissingJob creates a new missing items search job
func NewMissingJob(
	name string,
	cfg *config.SearchJobConfig,
	manager *jobs.Manager,
	logger *slog.Logger,
	testRun bool,
) *MissingJob {
	return &MissingJob{
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
func (j *MissingJob) Name() string {
	return j.name
}

// Enabled returns whether the job is enabled
func (j *MissingJob) Enabled() bool {
	return j.enabled
}

// Stats returns job statistics
func (j *MissingJob) Stats() jobs.JobStats {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return jobs.JobStats{
		Found:   j.lastFound,
		Removed: j.lastSearched,
	}
}

// Run executes the missing search job
func (j *MissingJob) Run(ctx context.Context) error {
	j.logger.Debug("starting missing items search job",
		"test_run", j.testRun,
		"min_days_between_searches", j.minDaysBetweenSearches,
		"max_concurrent_searches", j.maxConcurrentSearches,
	)

	found := 0
	searched := 0

	// Semaphore to limit concurrent searches
	searchSem := make(chan struct{}, j.maxConcurrentSearches)
	var wg sync.WaitGroup
	var searchMu sync.Mutex
	var errs []error

	// Search missing items in Sonarr instances
	sonarrClients, err := j.getSonarrClients()
	if err != nil {
		j.logger.Error("failed to get sonarr clients", "error", err)
		errs = append(errs, err)
	} else {
		for name, client := range sonarrClients {
			wg.Add(1)
			go func(instanceName string, sc *arrapi.SonarrClient) {
				defer wg.Done()

				f, s, err := j.searchMissingSonarr(ctx, instanceName, sc, searchSem)
				searchMu.Lock()
				found += f
				searched += s
				if err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", instanceName, err))
				}
				searchMu.Unlock()
			}(name, client)
		}
	}

	// Search missing items in Radarr instances
	radarrClients, err := j.getRadarrClients()
	if err != nil {
		j.logger.Error("failed to get radarr clients", "error", err)
		errs = append(errs, err)
	} else {
		for name, client := range radarrClients {
			wg.Add(1)
			go func(instanceName string, rc *arrapi.RadarrClient) {
				defer wg.Done()

				f, s, err := j.searchMissingRadarr(ctx, instanceName, rc, searchSem)
				searchMu.Lock()
				found += f
				searched += s
				if err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", instanceName, err))
				}
				searchMu.Unlock()
			}(name, client)
		}
	}

	wg.Wait()

	// Update stats
	j.mu.Lock()
	j.lastFound = found
	j.lastSearched = searched
	j.mu.Unlock()

	j.logger.Debug("missing items search completed",
		"found", found,
		"searches_triggered", searched,
		"errors", len(errs))

	if len(errs) > 0 {
		return fmt.Errorf("encountered %d errors during search: %v", len(errs), errs)
	}

	return nil
}

// searchMissingSonarr searches for missing episodes in a Sonarr instance
func (j *MissingJob) searchMissingSonarr(ctx context.Context, instanceName string, client *arrapi.SonarrClient, searchSem chan struct{}) (found int, searched int, err error) {
	logger := j.logger.With("instance", instanceName, "type", "sonarr")
	logger.Debug("searching for missing episodes")

	// Get all series
	allSeries, err := client.GetAllSeries(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get series: %w", err)
	}

	logger.Debug("retrieved series", "count", len(allSeries))

	for _, series := range allSeries {
		if !series.Monitored {
			continue
		}

		// Get episodes for this series
		episodes, err := client.GetEpisodes(ctx, series.ID)
		if err != nil {
			logger.Error("failed to get episodes", "series", series.Title, "error", err)
			continue
		}

		// Find missing episodes (monitored, no file, aired)
		var missingEpisodes []arrapi.Episode
		for _, ep := range episodes {
			if ep.Monitored && !ep.HasFile && !ep.AirDateUTC.IsZero() && ep.AirDateUTC.Before(time.Now()) {
				missingEpisodes = append(missingEpisodes, ep)
			}
		}

		// Filter out recently searched episodes
		eligibleEpisodes := j.filterRecentlySearchedEpisodes(missingEpisodes)

		// Extract episode IDs
		var missingEpisodeIDs []int
		for _, ep := range eligibleEpisodes {
			missingEpisodeIDs = append(missingEpisodeIDs, ep.ID)
		}

		if len(missingEpisodeIDs) > 0 {
			found += len(missingEpisodeIDs)
			logger.Debug("found missing episodes",
				"series", series.Title,
				"count", len(missingEpisodeIDs))

			if !j.testRun {
				// Acquire semaphore slot
				searchSem <- struct{}{}
				err := client.SearchEpisodes(ctx, missingEpisodeIDs)
				<-searchSem // Release slot

				if err != nil {
					logger.Error("failed to trigger search",
						"series", series.Title,
						"episode_count", len(missingEpisodeIDs),
						"error", err)
				} else {
					searched += len(missingEpisodeIDs)
					logger.Debug("triggered search",
						"series", series.Title,
						"episode_count", len(missingEpisodeIDs))
				}
			} else {
				logger.Debug("test run: would trigger search",
					"series", series.Title,
					"episode_count", len(missingEpisodeIDs))
			}
		}
	}

	return found, searched, nil
}

// filterRecentlySearchedMovies filters out movies that have been searched within minDays
func (j *MissingJob) filterRecentlySearchedMovies(movies []arrapi.Movie) []arrapi.Movie {
	if j.minDaysBetweenSearches <= 0 {
		return movies
	}

	now := time.Now()
	threshold := now.AddDate(0, 0, -j.minDaysBetweenSearches)

	var eligible []arrapi.Movie
	for _, movie := range movies {
		if movie.LastSearchTime == nil {
			// Never searched, eligible
			eligible = append(eligible, movie)
			continue
		}

		if movie.LastSearchTime.Before(threshold) {
			// Last search was more than minDays ago
			eligible = append(eligible, movie)
		} else {
			daysAgo := int(now.Sub(*movie.LastSearchTime).Hours() / 24)
			j.logger.Debug("skipping recently searched movie",
				"movie_id", movie.ID,
				"title", movie.Title,
				"last_search", movie.LastSearchTime.Format(time.RFC3339),
				"days_ago", daysAgo,
				"min_days", j.minDaysBetweenSearches,
			)
		}
	}
	return eligible
}

// filterRecentlySearchedEpisodes filters out episodes that have been searched within minDays
func (j *MissingJob) filterRecentlySearchedEpisodes(episodes []arrapi.Episode) []arrapi.Episode {
	if j.minDaysBetweenSearches <= 0 {
		return episodes
	}

	now := time.Now()
	threshold := now.AddDate(0, 0, -j.minDaysBetweenSearches)

	var eligible []arrapi.Episode
	for _, ep := range episodes {
		if ep.LastSearchTime == nil {
			// Never searched, eligible
			eligible = append(eligible, ep)
			continue
		}

		if ep.LastSearchTime.Before(threshold) {
			// Last search was more than minDays ago
			eligible = append(eligible, ep)
		} else {
			daysAgo := int(now.Sub(*ep.LastSearchTime).Hours() / 24)
			j.logger.Debug("skipping recently searched episode",
				"episode_id", ep.ID,
				"title", ep.Title,
				"last_search", ep.LastSearchTime.Format(time.RFC3339),
				"days_ago", daysAgo,
				"min_days", j.minDaysBetweenSearches,
			)
		}
	}
	return eligible
}

// searchMissingRadarr searches for missing movies in a Radarr instance
func (j *MissingJob) searchMissingRadarr(ctx context.Context, instanceName string, client *arrapi.RadarrClient, searchSem chan struct{}) (found int, searched int, err error) {
	logger := j.logger.With("instance", instanceName, "type", "radarr")
	logger.Debug("searching for missing movies")

	// Get all movies
	allMovies, err := client.GetAllMovies(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get movies: %w", err)
	}

	logger.Debug("retrieved movies", "count", len(allMovies))

	// Filter missing movies (monitored, no file, available)
	var missingMovies []arrapi.Movie
	for _, movie := range allMovies {
		if !movie.Monitored || movie.HasFile {
			continue
		}

		// Check if movie is available (released)
		if !movie.IsAvailable {
			continue
		}

		missingMovies = append(missingMovies, movie)
	}

	// Filter out recently searched movies
	eligibleMovies := j.filterRecentlySearchedMovies(missingMovies)

	for _, movie := range eligibleMovies {
		found++
		logger.Debug("found missing movie", "title", movie.Title, "year", movie.Year)

		if !j.testRun {
			// Acquire semaphore slot
			searchSem <- struct{}{}
			err := client.SearchMovie(ctx, movie.ID)
			<-searchSem // Release slot

			if err != nil {
				logger.Error("failed to trigger search",
					"movie", movie.Title,
					"year", movie.Year,
					"error", err)
			} else {
				searched++
				logger.Debug("triggered search", "movie", movie.Title, "year", movie.Year)
			}
		} else {
			logger.Debug("test run: would trigger search", "movie", movie.Title, "year", movie.Year)
		}
	}

	return found, searched, nil
}

// getSonarrClients retrieves all Sonarr clients from the manager
func (j *MissingJob) getSonarrClients() (map[string]*arrapi.SonarrClient, error) {
	clients := make(map[string]*arrapi.SonarrClient)

	// Access config to get Sonarr instance names
	for _, inst := range j.manager.GetConfig().Instances.Sonarr {
		if !inst.Enabled {
			continue
		}

		client, ok := j.manager.GetArrClient(inst.Name)
		if !ok {
			return nil, fmt.Errorf("sonarr client %s not found", inst.Name)
		}

		clients[inst.Name] = &arrapi.SonarrClient{Client: client}
	}

	return clients, nil
}

// getRadarrClients retrieves all Radarr clients from the manager
func (j *MissingJob) getRadarrClients() (map[string]*arrapi.RadarrClient, error) {
	clients := make(map[string]*arrapi.RadarrClient)

	// Access config to get Radarr instance names
	for _, inst := range j.manager.GetConfig().Instances.Radarr {
		if !inst.Enabled {
			continue
		}

		client, ok := j.manager.GetArrClient(inst.Name)
		if !ok {
			return nil, fmt.Errorf("radarr client %s not found", inst.Name)
		}

		clients[inst.Name] = &arrapi.RadarrClient{Client: client}
	}

	return clients, nil
}
