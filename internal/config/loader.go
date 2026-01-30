package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Configure viper for env vars
	v.SetEnvPrefix("DECLUTARR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Determine config file path
	if configPath == "" {
		// Check DECLUTARR_CONFIG env var
		configPath = os.Getenv("DECLUTARR_CONFIG")
	}
	if configPath == "" {
		// Try default locations
		defaultPaths := []string{"config.yaml", "config.yml", "/app/config.yaml"}
		for _, p := range defaultPaths {
			if _, err := os.Stat(p); err == nil {
				configPath = p
				break
			}
		}
	}

	// Read config file if found
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
		}
	}
	// If no file found, continue with defaults and env vars

	// Unmarshal into config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default values for all configuration options
func setDefaults(v *viper.Viper) {
	// General defaults
	v.SetDefault("general.log_level", "info")
	v.SetDefault("general.test_run", false)
	v.SetDefault("general.timer", 5*time.Minute)
	v.SetDefault("general.ssl_verification", true)
	v.SetDefault("general.request_timeout", 30*time.Second)
	v.SetDefault("general.private_tracker_handling", "keep")
	v.SetDefault("general.public_tracker_handling", "remove")

	// Job defaults
	v.SetDefault("job_defaults.max_strikes", 3)
	v.SetDefault("job_defaults.no_stalled", false)
	v.SetDefault("job_defaults.no_slow", false)
	v.SetDefault("job_defaults.no_active", false)
	v.SetDefault("job_defaults.no_uploading", false)
	v.SetDefault("job_defaults.permitted_attempts", 3)
	v.SetDefault("job_defaults.min_download_speed", 100.0) // KB/s
	v.SetDefault("job_defaults.min_time_left", 0*time.Second)
	v.SetDefault("job_defaults.min_ratio", 0.0)
	v.SetDefault("job_defaults.max_ratio", 0.0) // 0 = unlimited
	v.SetDefault("job_defaults.max_seed_time", 0*time.Second) // 0 = unlimited
	v.SetDefault("job_defaults.max_active_downloads", 0) // 0 = unlimited
	v.SetDefault("job_defaults.free_space_threshold", 10*1024*1024*1024) // 10GB
	v.SetDefault("job_defaults.apply_imported_action", true)
	v.SetDefault("job_defaults.apply_not_imported", false)
	v.SetDefault("job_defaults.apply_tags", false)

	// Individual jobs - all disabled by default
	v.SetDefault("jobs.remove_stalled.enabled", false)
	v.SetDefault("jobs.remove_slow.enabled", false)
	v.SetDefault("jobs.remove_failed_imports.enabled", false)
	v.SetDefault("jobs.remove_unmonitored.enabled", false)
	v.SetDefault("jobs.remove_orphans.enabled", false)
	v.SetDefault("jobs.remove_missing_files.enabled", false)
	v.SetDefault("jobs.tag_orphans.enabled", false)
	v.SetDefault("jobs.remove_metadata_failed.enabled", false)
	v.SetDefault("jobs.enforce_seeding_limits.enabled", false)
	v.SetDefault("jobs.manage_free_space.enabled", false)
	v.SetDefault("jobs.remove_duplicate_downloads.enabled", false)

	// Instances - empty by default
	v.SetDefault("instances.sonarr", []InstanceConfig{})
	v.SetDefault("instances.radarr", []InstanceConfig{})
	v.SetDefault("instances.lidarr", []InstanceConfig{})
	v.SetDefault("instances.readarr", []InstanceConfig{})

	// Download clients - empty by default
	v.SetDefault("download_clients.qbittorrent", []QbittorrentConfig{})
	v.SetDefault("download_clients.sabnzbd", []SabnzbdConfig{})
	v.SetDefault("download_clients.nzbget", []NzbgetConfig{})
}
