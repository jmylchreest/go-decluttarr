// Package version provides build-time version information for go-decluttarr.
//
// Variables in this package are set at build time using ldflags:
//
//	go build -ldflags "-X github.com/jmylchreest/go-decluttarr/internal/version.Version=1.0.0 ..."
package version

import (
	"fmt"
	"runtime"
)

// Build-time variables set via ldflags
var (
	// Version is the semantic version (e.g., "1.0.0" or "1.0.0-dev.5+abc123")
	Version = "dev"

	// Commit is the git commit SHA
	Commit = "unknown"

	// BuildDate is the UTC build timestamp in RFC3339 format
	BuildDate = "unknown"
)

// Info contains structured version information
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns the current version information
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a single-line version string
func String() string {
	return Version
}
