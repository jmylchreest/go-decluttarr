package removal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jmylchreest/go-decluttarr/internal/arrapi"
	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/jobs"
)

// MetadataDownloadingJob handles torrents waiting for swarm metadata, independently
// of library matching. Detection uses arr messages, not download-client state.
type MetadataDownloadingJob struct {
	name             string
	enabled, testRun bool
	maxStrikes       int
	patterns         []string
	manager          *jobs.Manager
	logger           *slog.Logger
	stats            jobs.JobStats
}

func NewMetadataDownloadingJob(name string, cfg *config.JobConfig, defaults *config.JobDefaultsConfig, manager *jobs.Manager, logger *slog.Logger, testRun bool) *MetadataDownloadingJob {
	maxStrikes := defaults.MaxStrikes
	if cfg.MaxStrikes != nil {
		maxStrikes = *cfg.MaxStrikes
	}
	patterns := cfg.MessagePatterns
	if len(patterns) == 0 {
		patterns = []string{"downloading metadata"}
	}
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern = strings.ToLower(strings.TrimSpace(pattern)); pattern != "" {
			normalized = append(normalized, pattern)
		}
	}
	return &MetadataDownloadingJob{name: name, enabled: cfg.Enabled, testRun: testRun, maxStrikes: maxStrikes, patterns: normalized, manager: manager, logger: logger.With("job", name)}
}
func (j *MetadataDownloadingJob) Name() string         { return j.name }
func (j *MetadataDownloadingJob) Enabled() bool        { return j.enabled }
func (j *MetadataDownloadingJob) Stats() jobs.JobStats { return j.stats }

func (j *MetadataDownloadingJob) matches(item arrapi.QueueItem) bool {
	if item.ID <= 0 || item.DownloadID == "" || !strings.EqualFold(item.Protocol, "torrent") {
		return false
	}
	status := strings.ToLower(item.Status)
	state := strings.ToLower(item.TrackedDownloadState)
	if status != "queued" && status != "downloading" {
		return false
	}
	if state != "" && state != "downloading" {
		return false
	}
	matchesText := func(text string) bool {
		text = strings.ToLower(text)
		for _, pattern := range j.patterns {
			if strings.Contains(text, pattern) {
				return true
			}
		}
		return false
	}
	if matchesText(item.ErrorMessage) {
		return true
	}
	for _, message := range item.StatusMessages {
		if matchesText(message.Title) {
			return true
		}
		for _, text := range message.Messages {
			if matchesText(text) {
				return true
			}
		}
	}
	return false
}

// Quoted components avoid collisions and keep this rule's strikes independent
// of other jobs, instances and download clients.
func (j *MetadataDownloadingJob) strikeKey(instance string, item arrapi.QueueItem) string {
	return fmt.Sprintf("%q/%q/%q/%q", j.name, instance, item.DownloadClient, strings.ToLower(item.DownloadID))
}

func (j *MetadataDownloadingJob) Run(ctx context.Context) error {
	j.stats = jobs.JobStats{}
	if j.maxStrikes < 1 {
		return fmt.Errorf("metadata downloading requires at least one strike")
	}
	queues, err := j.manager.GetAllQueues(ctx)
	if err != nil {
		return fmt.Errorf("fetch metadata queues: %w", err)
	}
	strikes := j.manager.GetStrikesHandler()
	seen := make(map[string]bool)
	var errs []error
	for instance, queue := range queues {
		for _, item := range queue {
			if err := ctx.Err(); err != nil {
				return err
			}
			key := j.strikeKey(instance, item)
			if j.manager.IsIgnoredDownloadClient(item.DownloadClient) || !j.matches(item) {
				strikes.Reset(key)
				continue
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			// Protection applies before accumulating strikes as well as before mutation.
			action := j.manager.GetRemovalAction(ctx, item.DownloadID)
			if action != "remove" && action != "tag" {
				strikes.Reset(key)
				continue
			}
			j.stats.Found++
			count := strikes.Add(key, j.name, item.Title)
			j.logger.Debug("download waiting for metadata", "instance", instance, "download_id", item.DownloadID, "strikes", count, "max_strikes", j.maxStrikes)
			if count < j.maxStrikes {
				continue
			}
			if j.testRun {
				j.logger.Info("[TEST RUN] would handle metadata download", "instance", instance, "download_id", item.DownloadID, "action", action)
				continue
			}
			if action == "tag" {
				err = j.manager.ApplyObsoleteTag(ctx, item.DownloadID)
			} else {
				client, ok := j.manager.GetArrClient(instance)
				if !ok {
					errs = append(errs, fmt.Errorf("arr client not found: %s", instance))
					continue
				}
				// Blocklist the unavailable release and allow the owning arr to seek a replacement.
				err = client.DeleteQueueItem(ctx, item.ID, arrapi.DeleteOptions{RemoveFromClient: true, Blocklist: true, SkipRedownload: false})
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s metadata download %d: %w", instance, item.ID, err))
				continue
			}
			strikes.Reset(key)
			j.stats.Removed++
			j.logger.Info("handled metadata download", "instance", instance, "download_id", item.DownloadID, "action", action)
		}
	}
	return errors.Join(errs...)
}
