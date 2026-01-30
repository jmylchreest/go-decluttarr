package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/jmylchreest/slog-logfilter"
)

// Setup configures the logger with the given level and format.
// Formats: "json" (default, recommended for k8s), "text" (logfmt style)
// For pretty output, pipe JSON through humanlog: kubectl logs -f app | humanlog
func Setup(logLevel string, format string) *slog.Logger {
	level := parseLevel(logLevel)

	opts := []logfilter.Option{
		logfilter.WithLevel(level),
		logfilter.WithOutput(os.Stdout),
	}

	if format == "text" {
		opts = append(opts, logfilter.WithFormat("text"))
	} else {
		opts = append(opts, logfilter.WithFormat("json"))
	}

	logger := logfilter.New(opts...)
	slog.SetDefault(logger)
	return logger
}

func SetLevel(level slog.Level) {
	logfilter.SetLevel(level)
}

func AddJobFilter(jobName string) {
	logfilter.AddFilter(logfilter.LogFilter{
		Type:    "job",
		Pattern: jobName,
		Level:   "debug",
		Enabled: true,
	})
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
