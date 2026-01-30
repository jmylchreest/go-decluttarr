package config

import (
	"fmt"
	"strings"
	"time"
)

// Validate checks the configuration for errors and inconsistencies
func (c *Config) Validate() error {
	// Validate general settings
	if err := c.validateGeneral(); err != nil {
		return fmt.Errorf("general config: %w", err)
	}

	// Validate job defaults
	if err := c.validateJobDefaults(); err != nil {
		return fmt.Errorf("job defaults: %w", err)
	}

	// Validate instances
	if err := c.validateInstances(); err != nil {
		return fmt.Errorf("instances: %w", err)
	}

	// Validate download clients
	if err := c.validateDownloadClients(); err != nil {
		return fmt.Errorf("download clients: %w", err)
	}

	// Ensure at least one instance is configured
	hasInstance := len(c.Instances.Sonarr) > 0 ||
		len(c.Instances.Radarr) > 0 ||
		len(c.Instances.Lidarr) > 0 ||
		len(c.Instances.Readarr) > 0
	if !hasInstance {
		return fmt.Errorf("at least one instance must be configured")
	}

	return nil
}

func (c *Config) validateGeneral() error {
	// Validate log level
	validLogLevels := []string{"debug", "info", "warn", "error"}
	logLevel := strings.ToLower(c.General.LogLevel)
	valid := false
	for _, level := range validLogLevels {
		if logLevel == level {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("log_level must be one of: %s", strings.Join(validLogLevels, ", "))
	}

	// Validate timer
	if c.General.Timer < 1*time.Minute {
		return fmt.Errorf("timer must be at least 1 minute")
	}
	if c.General.Timer > 24*time.Hour {
		return fmt.Errorf("timer must not exceed 24 hours")
	}

	// Validate request timeout
	if c.General.RequestTimeout < 1*time.Second {
		return fmt.Errorf("request_timeout must be at least 1 second")
	}
	if c.General.RequestTimeout > 5*time.Minute {
		return fmt.Errorf("request_timeout must not exceed 5 minutes")
	}

	// Validate tracker handling
	validHandling := []string{"keep", "remove", "pause"}
	if !isValidChoice(c.General.PrivateTrackerHandling, validHandling) {
		return fmt.Errorf("private_tracker_handling must be one of: %s", strings.Join(validHandling, ", "))
	}
	if !isValidChoice(c.General.PublicTrackerHandling, validHandling) {
		return fmt.Errorf("public_tracker_handling must be one of: %s", strings.Join(validHandling, ", "))
	}

	return nil
}

func (c *Config) validateJobDefaults() error {
	// Validate max strikes
	if c.JobDefaults.MaxStrikes < 1 {
		return fmt.Errorf("max_strikes must be at least 1")
	}

	// Validate permitted attempts
	if c.JobDefaults.PermittedAttempts < 0 {
		return fmt.Errorf("permitted_attempts cannot be negative")
	}

	// Validate min download speed
	if c.JobDefaults.MinDownloadSpeed < 0 {
		return fmt.Errorf("min_download_speed cannot be negative")
	}

	// Validate ratios
	if c.JobDefaults.MinRatio < 0 {
		return fmt.Errorf("min_ratio cannot be negative")
	}
	if c.JobDefaults.MaxRatio < 0 {
		return fmt.Errorf("max_ratio cannot be negative")
	}
	if c.JobDefaults.MaxRatio > 0 && c.JobDefaults.MinRatio > c.JobDefaults.MaxRatio {
		return fmt.Errorf("min_ratio cannot be greater than max_ratio")
	}

	// Validate seed time
	if c.JobDefaults.MaxSeedTime < 0 {
		return fmt.Errorf("max_seed_time cannot be negative")
	}

	// Validate max active downloads
	if c.JobDefaults.MaxActiveDownloads < 0 {
		return fmt.Errorf("max_active_downloads cannot be negative")
	}

	// Validate free space threshold
	if c.JobDefaults.FreeSpaceThreshold < 0 {
		return fmt.Errorf("free_space_threshold cannot be negative")
	}

	return nil
}

func (c *Config) validateInstances() error {
	// Track instance names to ensure uniqueness
	instanceNames := make(map[string]bool)

	// Validate all instance types
	for _, instance := range c.Instances.Sonarr {
		if err := validateInstance(instance, "sonarr", instanceNames); err != nil {
			return err
		}
	}
	for _, instance := range c.Instances.Radarr {
		if err := validateInstance(instance, "radarr", instanceNames); err != nil {
			return err
		}
	}
	for _, instance := range c.Instances.Lidarr {
		if err := validateInstance(instance, "lidarr", instanceNames); err != nil {
			return err
		}
	}
	for _, instance := range c.Instances.Readarr {
		if err := validateInstance(instance, "readarr", instanceNames); err != nil {
			return err
		}
	}

	return nil
}

func validateInstance(instance InstanceConfig, instanceType string, instanceNames map[string]bool) error {
	// Validate name
	if instance.Name == "" {
		return fmt.Errorf("%s instance must have a name", instanceType)
	}

	// Check for duplicate names
	if instanceNames[instance.Name] {
		return fmt.Errorf("duplicate instance name: %s", instance.Name)
	}
	instanceNames[instance.Name] = true

	// Validate URL
	if instance.URL == "" {
		return fmt.Errorf("%s instance '%s': URL is required", instanceType, instance.Name)
	}
	if !strings.HasPrefix(instance.URL, "http://") && !strings.HasPrefix(instance.URL, "https://") {
		return fmt.Errorf("%s instance '%s': URL must start with http:// or https://", instanceType, instance.Name)
	}

	// Validate API key
	if instance.APIKey == "" {
		return fmt.Errorf("%s instance '%s': API key is required", instanceType, instance.Name)
	}

	// Validate that enabled_jobs and disabled_jobs don't overlap
	enabledMap := make(map[string]bool)
	for _, job := range instance.EnabledJobs {
		enabledMap[job] = true
	}
	for _, job := range instance.DisabledJobs {
		if enabledMap[job] {
			return fmt.Errorf("%s instance '%s': job '%s' cannot be both enabled and disabled", instanceType, instance.Name, job)
		}
	}

	return nil
}

func (c *Config) validateDownloadClients() error {
	// Track client names to ensure uniqueness
	clientNames := make(map[string]bool)

	// Validate qBittorrent clients
	for _, client := range c.DownloadClients.Qbittorrent {
		if err := validateQbittorrent(client, clientNames); err != nil {
			return err
		}
	}

	// Validate SABnzbd clients
	for _, client := range c.DownloadClients.Sabnzbd {
		if err := validateSabnzbd(client, clientNames); err != nil {
			return err
		}
	}

	// Validate NZBGet clients
	for _, client := range c.DownloadClients.Nzbget {
		if err := validateNzbget(client, clientNames); err != nil {
			return err
		}
	}

	return nil
}

func validateQbittorrent(client QbittorrentConfig, clientNames map[string]bool) error {
	if client.Name == "" {
		return fmt.Errorf("qbittorrent client must have a name")
	}
	if clientNames[client.Name] {
		return fmt.Errorf("duplicate download client name: %s", client.Name)
	}
	clientNames[client.Name] = true

	if client.URL == "" {
		return fmt.Errorf("qbittorrent client '%s': URL is required", client.Name)
	}
	if !strings.HasPrefix(client.URL, "http://") && !strings.HasPrefix(client.URL, "https://") {
		return fmt.Errorf("qbittorrent client '%s': URL must start with http:// or https://", client.Name)
	}

	return nil
}

func validateSabnzbd(client SabnzbdConfig, clientNames map[string]bool) error {
	if client.Name == "" {
		return fmt.Errorf("sabnzbd client must have a name")
	}
	if clientNames[client.Name] {
		return fmt.Errorf("duplicate download client name: %s", client.Name)
	}
	clientNames[client.Name] = true

	if client.URL == "" {
		return fmt.Errorf("sabnzbd client '%s': URL is required", client.Name)
	}
	if !strings.HasPrefix(client.URL, "http://") && !strings.HasPrefix(client.URL, "https://") {
		return fmt.Errorf("sabnzbd client '%s': URL must start with http:// or https://", client.Name)
	}

	if client.APIKey == "" {
		return fmt.Errorf("sabnzbd client '%s': API key is required", client.Name)
	}

	return nil
}

func validateNzbget(client NzbgetConfig, clientNames map[string]bool) error {
	if client.Name == "" {
		return fmt.Errorf("nzbget client must have a name")
	}
	if clientNames[client.Name] {
		return fmt.Errorf("duplicate download client name: %s", client.Name)
	}
	clientNames[client.Name] = true

	if client.URL == "" {
		return fmt.Errorf("nzbget client '%s': URL is required", client.Name)
	}
	if !strings.HasPrefix(client.URL, "http://") && !strings.HasPrefix(client.URL, "https://") {
		return fmt.Errorf("nzbget client '%s': URL must start with http:// or https://", client.Name)
	}

	return nil
}

// isValidChoice checks if a value is in a list of valid choices
func isValidChoice(value string, choices []string) bool {
	value = strings.ToLower(value)
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
