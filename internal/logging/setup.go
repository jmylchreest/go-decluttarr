package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/jmylchreest/slog-logfilter"
)

func Setup(logLevel string, format string) *slog.Logger {
	level := parseLevel(logLevel)

	opts := []logfilter.Option{
		logfilter.WithLevel(level),
		logfilter.WithOutput(os.Stdout),
	}

	if format == "json" {
		opts = append(opts, logfilter.WithFormat("json"))
	} else {
		opts = append(opts, logfilter.WithFormat("text"))
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
