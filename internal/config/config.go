package config

import "time"

// Config represents the complete application configuration
type Config struct {
	General         GeneralConfig         `mapstructure:"general"`
	JobDefaults     JobDefaultsConfig     `mapstructure:"job_defaults"`
	Jobs            JobsConfig            `mapstructure:"jobs"`
	Instances       InstancesConfig       `mapstructure:"instances"`
	DownloadClients DownloadClientsConfig `mapstructure:"download_clients"`
}

// GeneralConfig contains global application settings
type GeneralConfig struct {
	LogLevel               string        `mapstructure:"log_level"`
	TestRun                bool          `mapstructure:"test_run"`
	Timer                  time.Duration `mapstructure:"timer"`
	SSLVerification        bool          `mapstructure:"ssl_verification"`
	RequestTimeout         time.Duration `mapstructure:"request_timeout"`
	PrivateTrackerHandling string        `mapstructure:"private_tracker_handling"`
	PublicTrackerHandling  string        `mapstructure:"public_tracker_handling"`
	IgnoreDownloadClients  []string      `mapstructure:"ignore_download_clients"`
	ObsoleteTag            string        `mapstructure:"obsolete_tag"`
	ProtectedTag           string        `mapstructure:"protected_tag"`
}

// JobDefaultsConfig contains default settings for all jobs
type JobDefaultsConfig struct {
	MaxStrikes          int           `mapstructure:"max_strikes"`
	NoStalled           bool          `mapstructure:"no_stalled"`
	NoSlow              bool          `mapstructure:"no_slow"`
	NoActive            bool          `mapstructure:"no_active"`
	NoUploading         bool          `mapstructure:"no_uploading"`
	PermittedAttempts   int           `mapstructure:"permitted_attempts"`
	MinDownloadSpeed    float64       `mapstructure:"min_download_speed"`
	MinTimeLeft         time.Duration `mapstructure:"min_time_left"`
	MinRatio            float64       `mapstructure:"min_ratio"`
	MaxRatio            float64       `mapstructure:"max_ratio"`
	MaxSeedTime         time.Duration `mapstructure:"max_seed_time"`
	MaxActiveDownloads  int           `mapstructure:"max_active_downloads"`
	FreeSpaceThreshold  int64         `mapstructure:"free_space_threshold"`
	ApplyImportedAction bool          `mapstructure:"apply_imported_action"`
	ApplyNotImported    bool          `mapstructure:"apply_not_imported"`
	ApplyTags           bool          `mapstructure:"apply_tags"`
}

// JobsConfig contains individual job configurations
type JobsConfig struct {
	RemoveStalled            JobConfig               `mapstructure:"remove_stalled"`
	RemoveSlow               JobConfig               `mapstructure:"remove_slow"`
	RemoveFailedImports      JobConfig               `mapstructure:"remove_failed_imports"`
	RemoveFailedDownloads    JobConfig               `mapstructure:"remove_failed_downloads"`
	RemoveUnmonitored        JobConfig               `mapstructure:"remove_unmonitored"`
	RemoveOrphans            JobConfig               `mapstructure:"remove_orphans"`
	RemoveMissingFiles       JobConfig               `mapstructure:"remove_missing_files"`
	RemoveBadFiles           JobConfig               `mapstructure:"remove_bad_files"`
	TagOrphans               JobConfig               `mapstructure:"tag_orphans"`
	RemoveMetadataFailed     JobConfig               `mapstructure:"remove_metadata_failed"`
	EnforceSeedingLimits     JobConfig               `mapstructure:"enforce_seeding_limits"`
	ManageFreeSpace          JobConfig               `mapstructure:"manage_free_space"`
	RemoveDuplicateDownloads JobConfig               `mapstructure:"remove_duplicate_downloads"`
	RemoveDoneSeeding        RemoveDoneSeedingConfig `mapstructure:"remove_done_seeding"`
	SearchMissing            SearchJobConfig         `mapstructure:"search_missing"`
	SearchUnmetCutoff        SearchJobConfig         `mapstructure:"search_unmet_cutoff"`
}

// JobConfig represents configuration for a specific job
type JobConfig struct {
	Enabled             bool          `mapstructure:"enabled"`
	MaxStrikes          *int          `mapstructure:"max_strikes"`
	NoStalled           *bool         `mapstructure:"no_stalled"`
	NoSlow              *bool         `mapstructure:"no_slow"`
	NoActive            *bool         `mapstructure:"no_active"`
	NoUploading         *bool         `mapstructure:"no_uploading"`
	PermittedAttempts   *int          `mapstructure:"permitted_attempts"`
	MinDownloadSpeed    *float64      `mapstructure:"min_download_speed"`
	MinTimeLeft         *time.Duration `mapstructure:"min_time_left"`
	MinRatio            *float64      `mapstructure:"min_ratio"`
	MaxRatio            *float64      `mapstructure:"max_ratio"`
	MaxSeedTime         *time.Duration `mapstructure:"max_seed_time"`
	MaxActiveDownloads  *int          `mapstructure:"max_active_downloads"`
	FreeSpaceThreshold  *int64        `mapstructure:"free_space_threshold"`
	ApplyImportedAction *bool         `mapstructure:"apply_imported_action"`
	ApplyNotImported    *bool         `mapstructure:"apply_not_imported"`
	ApplyTags           *bool         `mapstructure:"apply_tags"`
	TagsToApply         []string      `mapstructure:"tags_to_apply"`
	MessagePatterns     []string      `mapstructure:"message_patterns"`
	KeepArchives        *bool         `mapstructure:"keep_archives"`
}

// SearchJobConfig represents configuration for search jobs
type SearchJobConfig struct {
	Enabled                bool `mapstructure:"enabled"`
	MinDaysBetweenSearches int  `mapstructure:"min_days_between_searches"`
	MaxConcurrentSearches  int  `mapstructure:"max_concurrent_searches"`
}

// RemoveDoneSeedingConfig represents configuration for remove_done_seeding job
type RemoveDoneSeedingConfig struct {
	Enabled          bool     `mapstructure:"enabled"`
	TargetTags       []string `mapstructure:"target_tags"`
	TargetCategories []string `mapstructure:"target_categories"`
}

// InstancesConfig contains all *arr instance configurations
type InstancesConfig struct {
	Sonarr   []InstanceConfig `mapstructure:"sonarr"`
	Radarr   []InstanceConfig `mapstructure:"radarr"`
	Lidarr   []InstanceConfig `mapstructure:"lidarr"`
	Readarr  []InstanceConfig `mapstructure:"readarr"`
	Whisparr []InstanceConfig `mapstructure:"whisparr"`
}

// InstanceConfig represents a single *arr instance
type InstanceConfig struct {
	Name                   string   `mapstructure:"name"`
	URL                    string   `mapstructure:"url"`
	APIKey                 string   `mapstructure:"api_key"`
	Enabled                bool     `mapstructure:"enabled"`
	EnabledJobs            []string `mapstructure:"enabled_jobs"`
	DisabledJobs           []string `mapstructure:"disabled_jobs"`
	ProtectedTags          []string `mapstructure:"protected_tags"`
	IgnoreTags             []string `mapstructure:"ignore_tags"`
	OnlyTags               []string `mapstructure:"only_tags"`
	DownloadClientPriority []string `mapstructure:"download_client_priority"`
}

// DownloadClientsConfig contains all download client configurations
type DownloadClientsConfig struct {
	Qbittorrent []QbittorrentConfig `mapstructure:"qbittorrent"`
	Sabnzbd     []SabnzbdConfig     `mapstructure:"sabnzbd"`
	Nzbget      []NzbgetConfig      `mapstructure:"nzbget"`
}

// QbittorrentConfig represents a qBittorrent client
type QbittorrentConfig struct {
	Name     string `mapstructure:"name"`
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Enabled  bool   `mapstructure:"enabled"`
}

// SabnzbdConfig represents a SABnzbd client
type SabnzbdConfig struct {
	Name    string `mapstructure:"name"`
	URL     string `mapstructure:"url"`
	APIKey  string `mapstructure:"api_key"`
	Enabled bool   `mapstructure:"enabled"`
}

// NzbgetConfig represents an NZBGet client
type NzbgetConfig struct {
	Name     string `mapstructure:"name"`
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Enabled  bool   `mapstructure:"enabled"`
}
