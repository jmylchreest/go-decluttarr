package logging

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jmylchreest/slog-logfilter"
)

// Setup replaces global logger state, so these tests intentionally run serially.
func captureLogs(t *testing.T, run func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "logs")
	if err != nil {
		t.Fatal(err)
	}
	previousOutput, previousLogger := os.Stdout, slog.Default()
	os.Stdout = file
	t.Cleanup(func() { os.Stdout = previousOutput; slog.SetDefault(previousLogger); file.Close() })
	run()
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSetupFormatsAndDefaultLogger(t *testing.T) {
	for _, format := range []string{"json", "text", "unknown"} {
		t.Run(format, func(t *testing.T) {
			output := captureLogs(t, func() {
				logger := Setup("info", format)
				if slog.Default() != logger {
					t.Fatal("default logger not installed")
				}
				logger.Debug("suppressed")
				logger.With("job", "cleanup").Info("visible")
			})
			if strings.Contains(output, "suppressed") {
				t.Fatal("debug emitted at info")
			}
			if format == "text" {
				if !strings.Contains(output, "msg=visible") || !strings.Contains(output, "job=cleanup") {
					t.Fatalf("bad text output: %s", output)
				}
			} else {
				var record map[string]any
				if err := json.Unmarshal([]byte(output), &record); err != nil {
					t.Fatal(err)
				}
				if record["msg"] != "visible" || record["job"] != "cleanup" || record["level"] != "INFO" {
					t.Fatalf("bad record: %v", record)
				}
			}
		})
	}
}

func TestRuntimeLevelAndJobFilterReachChildLoggers(t *testing.T) {
	output := captureLogs(t, func() {
		logger := Setup("info", "json")
		selected := logger.With("job", "selected")
		other := logger.With("job", "other")
		AddJobFilter("selected")
		selected.Debug("selected-debug")
		other.Debug("other-suppressed")
		SetLevel(slog.LevelDebug)
		other.Debug("global-debug")
		SetLevel(slog.LevelError)
		other.Info("info-suppressed")
		other.Error("global-error")
	})
	for _, message := range []string{"selected-debug", "global-debug", "global-error"} {
		if !strings.Contains(output, message) {
			t.Errorf("missing %s", message)
		}
	}
	for _, message := range []string{"other-suppressed", "info-suppressed"} {
		if strings.Contains(output, message) {
			t.Errorf("unexpected %s", message)
		}
	}
}

func TestApplicationUpdatesDoNotAffectIndependentLoggers(t *testing.T) {
	output := captureLogs(t, func() {
		application := Setup("info", "json")
		independent := logfilter.New(logfilter.WithOutput(os.Stdout), logfilter.WithLevel(slog.LevelError))
		SetLevel(slog.LevelDebug)
		AddJobFilter("selected")
		application.Debug("application-debug")
		independent.With("job", "selected").Debug("independent-suppressed")
		independent.Info("independent-info-suppressed")
		independent.Error("independent-error")
	})
	if !strings.Contains(output, "application-debug") || !strings.Contains(output, "independent-error") {
		t.Fatal("expected logs missing")
	}
	if strings.Contains(output, "independent-suppressed") || strings.Contains(output, "independent-info-suppressed") {
		t.Fatal("application settings leaked to independent logger")
	}
}

func TestSetupCreatesIndependentLoggerFamilies(t *testing.T) {
	output := captureLogs(t, func() {
		first := Setup("error", "json")
		Setup("info", "json")
		SetLevel(slog.LevelDebug)
		first.Debug("old-family-suppressed")
		slog.Debug("new-family-debug")
	})
	if strings.Contains(output, "old-family-suppressed") || !strings.Contains(output, "new-family-debug") {
		t.Fatal("logger family isolation failed")
	}
}
