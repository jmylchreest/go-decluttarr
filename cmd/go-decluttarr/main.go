package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jmylchreest/go-decluttarr/internal/arrapi"
	"github.com/jmylchreest/go-decluttarr/internal/config"
	"github.com/jmylchreest/go-decluttarr/internal/downloadclient"
	"github.com/jmylchreest/go-decluttarr/internal/jobs"
	"github.com/jmylchreest/go-decluttarr/internal/jobs/removal"
	"github.com/jmylchreest/go-decluttarr/internal/logging"
	"github.com/jmylchreest/go-decluttarr/internal/version"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "", "Path to config file (default: ./config.yaml or /app/config.yaml)")
	dataDir := flag.String("data", "./data", "Directory for persistent data (strikes, etc.)")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		info := version.Get()
		fmt.Printf("go-decluttarr %s\n", info.Version)
		fmt.Printf("  Commit:     %s\n", info.Commit)
		fmt.Printf("  Built:      %s\n", info.BuildDate)
		fmt.Printf("  Go version: %s\n", info.GoVersion)
		fmt.Printf("  OS/Arch:    %s\n", info.Platform)
		os.Exit(0)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Setup logging - env vars override config
	logLevel := cfg.General.LogLevel
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		logLevel = envLevel
	}
	logFormat := "json" // default to JSON for k8s/production
	if envFormat := os.Getenv("LOG_FORMAT"); envFormat != "" {
		logFormat = envFormat
	}
	logger := logging.Setup(logLevel, logFormat)
	info := version.Get()
	logger.Info("starting go-decluttarr",
		"version", info.Version,
		"commit", info.Commit,
		"built", info.BuildDate,
		"data_dir", *dataDir,
	)

	// Create manager with strikes persistence
	strikesPath := filepath.Join(*dataDir, "strikes.json")
	manager := jobs.NewManager(cfg, logger, strikesPath)
	defer manager.Close()

	registerAllJobs(manager, cfg, logger)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Main loop
	ticker := time.NewTicker(cfg.General.Timer)
	defer ticker.Stop()

	// Run immediately on startup
	runCycle(ctx, manager, logger, cfg.General.TestRun)

	for {
		select {
		case <-ticker.C:
			runCycle(ctx, manager, logger, cfg.General.TestRun)
		case <-sigChan:
			logger.Info("shutdown signal received")
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

func runCycle(ctx context.Context, manager *jobs.Manager, logger *slog.Logger, testRun bool) {
	if testRun {
		logger.Info("running in TEST MODE - no changes will be made")
	}
	if err := manager.RunAll(ctx); err != nil {
		logger.Error("cycle had errors", "error", err)
		// Continue running - don't exit!
	}
}

func registerAllJobs(manager *jobs.Manager, cfg *config.Config, logger *slog.Logger) {
	// Register arr clients (Sonarr/Radarr use v3, Lidarr/Readarr use v1)
	for _, inst := range cfg.Instances.Sonarr {
		client := arrapi.NewClient(arrapi.ClientConfig{
			Name:       inst.Name,
			BaseURL:    inst.URL,
			APIKey:     inst.APIKey,
			APIVersion: "v3",
			Timeout:    cfg.General.RequestTimeout,
			Logger:     logger,
		})
		manager.RegisterArrClient(inst.Name, client)
		logger.Debug("registered sonarr instance", "name", inst.Name, "url", inst.URL, "api", "v3")
	}
	for _, inst := range cfg.Instances.Radarr {
		client := arrapi.NewClient(arrapi.ClientConfig{
			Name:       inst.Name,
			BaseURL:    inst.URL,
			APIKey:     inst.APIKey,
			APIVersion: "v3",
			Timeout:    cfg.General.RequestTimeout,
			Logger:     logger,
		})
		manager.RegisterArrClient(inst.Name, client)
		logger.Debug("registered radarr instance", "name", inst.Name, "url", inst.URL, "api", "v3")
	}
	for _, inst := range cfg.Instances.Lidarr {
		client := arrapi.NewClient(arrapi.ClientConfig{
			Name:       inst.Name,
			BaseURL:    inst.URL,
			APIKey:     inst.APIKey,
			APIVersion: "v1",
			Timeout:    cfg.General.RequestTimeout,
			Logger:     logger,
		})
		manager.RegisterArrClient(inst.Name, client)
		logger.Debug("registered lidarr instance", "name", inst.Name, "url", inst.URL, "api", "v1")
	}
	for _, inst := range cfg.Instances.Readarr {
		client := arrapi.NewClient(arrapi.ClientConfig{
			Name:       inst.Name,
			BaseURL:    inst.URL,
			APIKey:     inst.APIKey,
			APIVersion: "v1",
			Timeout:    cfg.General.RequestTimeout,
			Logger:     logger,
		})
		manager.RegisterArrClient(inst.Name, client)
		logger.Debug("registered readarr instance", "name", inst.Name, "url", inst.URL, "api", "v1")
	}
	for _, inst := range cfg.Instances.Whisparr {
		client := arrapi.NewClient(arrapi.ClientConfig{
			Name:       inst.Name,
			BaseURL:    inst.URL,
			APIKey:     inst.APIKey,
			APIVersion: "v3",
			Timeout:    cfg.General.RequestTimeout,
			Logger:     logger,
		})
		manager.RegisterArrClient(inst.Name, client)
		logger.Debug("registered whisparr instance", "name", inst.Name, "url", inst.URL, "api", "v3")
	}

	// Register download clients
	for _, dc := range cfg.DownloadClients.Qbittorrent {
		client, err := downloadclient.NewQBittorrentClient(downloadclient.QBittorrentConfig{
			BaseURL:  dc.URL,
			Username: dc.Username,
			Password: dc.Password,
			Timeout:  cfg.General.RequestTimeout,
			Logger:   logger,
		})
		if err != nil {
			logger.Error("failed to create qbittorrent client", "name", dc.Name, "error", err)
			continue
		}
		manager.RegisterDownloadClient(dc.Name, client)
		logger.Debug("registered qbittorrent client", "name", dc.Name, "url", dc.URL)
	}

	// Register removal jobs - all using Pattern 1: (name, cfg, defaults, manager, logger, testRun)
	if cfg.Jobs.RemoveStalled.Enabled {
		job := removal.NewStalledJob("remove_stalled", &cfg.Jobs.RemoveStalled, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveFailedImports.Enabled {
		job := removal.NewFailedImportsJob("remove_failed_imports", &cfg.Jobs.RemoveFailedImports, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveFailedDownloads.Enabled {
		job := removal.NewFailedDownloadsJob("remove_failed_downloads", &cfg.Jobs.RemoveFailedDownloads, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveOrphans.Enabled {
		job := removal.NewOrphansJob("remove_orphans", &cfg.Jobs.RemoveOrphans, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveMissingFiles.Enabled {
		job := removal.NewMissingFilesJob("remove_missing_files", &cfg.Jobs.RemoveMissingFiles, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveUnmonitored.Enabled {
		job := removal.NewUnmonitoredJob("remove_unmonitored", &cfg.Jobs.RemoveUnmonitored, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveSlow.Enabled {
		job := removal.NewSlowDownloadJob("remove_slow", &cfg.Jobs.RemoveSlow, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveBadFiles.Enabled {
		job := removal.NewBadFilesJob("remove_bad_files", &cfg.Jobs.RemoveBadFiles, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveMetadataFailed.Enabled {
		job := removal.NewMetadataMissingJob("remove_metadata_failed", &cfg.Jobs.RemoveMetadataFailed, &cfg.JobDefaults, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}
	if cfg.Jobs.RemoveDoneSeeding.Enabled {
		job := removal.NewDoneSeedingJob("remove_done_seeding", &cfg.Jobs.RemoveDoneSeeding, manager, logger, cfg.General.TestRun)
		manager.RegisterJob(job)
	}

	logger.Debug("initialization complete",
		"arr_instances", len(cfg.Instances.Sonarr)+len(cfg.Instances.Radarr)+len(cfg.Instances.Lidarr)+len(cfg.Instances.Readarr)+len(cfg.Instances.Whisparr),
		"download_clients", len(cfg.DownloadClients.Qbittorrent)+len(cfg.DownloadClients.Sabnzbd)+len(cfg.DownloadClients.Nzbget),
	)
}
