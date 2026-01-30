package arrapi

// SystemStatus represents the *arr application status
type SystemStatus struct {
	AppName                 string `json:"appName"`
	InstanceName            string `json:"instanceName"`
	Version                 string `json:"version"`
	BuildTime               string `json:"buildTime"`
	IsDebug                 bool   `json:"isDebug"`
	IsProduction            bool   `json:"isProduction"`
	IsAdmin                 bool   `json:"isAdmin"`
	IsUserInteractive       bool   `json:"isUserInteractive"`
	StartupPath             string `json:"startupPath"`
	AppData                 string `json:"appData"`
	OsName                  string `json:"osName"`
	OsVersion               string `json:"osVersion"`
	IsMonoRuntime           bool   `json:"isMonoRuntime"`
	IsMono                  bool   `json:"isMono"`
	IsLinux                 bool   `json:"isLinux"`
	IsOsx                   bool   `json:"isOsx"`
	IsWindows               bool   `json:"isWindows"`
	IsDocker                bool   `json:"isDocker"`
	Mode                    string `json:"mode"`
	Branch                  string `json:"branch"`
	Authentication          string `json:"authentication"`
	SqliteVersion           string `json:"sqliteVersion"`
	MigrationVersion        int    `json:"migrationVersion"`
	UrlBase                 string `json:"urlBase"`
	RuntimeVersion          string `json:"runtimeVersion"`
	RuntimeName             string `json:"runtimeName"`
	StartTime               string `json:"startTime"`
	PackageVersion          string `json:"packageVersion"`
	PackageAuthor           string `json:"packageAuthor"`
	PackageUpdateMechanism  string `json:"packageUpdateMechanism"`
}

// CutoffUnmetResponse represents the paginated response from the wanted/cutoff API
type CutoffUnmetResponse struct {
	Page         int                `json:"page"`
	PageSize     int                `json:"pageSize"`
	TotalRecords int                `json:"totalRecords"`
	Records      []CutoffUnmetItem  `json:"records"`
}

// CutoffUnmetItem represents an item that doesn't meet quality cutoff
// This is used for both Sonarr (episodes) and Radarr (movies)
type CutoffUnmetItem struct {
	// Common fields
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Monitored bool   `json:"monitored"`

	// Sonarr-specific fields
	SeriesID      *int `json:"seriesId,omitempty"`
	EpisodeFileID *int `json:"episodeFileId,omitempty"`
	SeasonNumber  *int `json:"seasonNumber,omitempty"`
	EpisodeNumber *int `json:"episodeNumber,omitempty"`

	// Radarr-specific fields
	MovieID *int `json:"movieId,omitempty"`
}

