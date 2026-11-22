package logging

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

	"crusoe-registry-pruner/internal"
	"crusoe-registry-pruner/internal/config"
)

func InitializeDefault() {
	cfg := config.Log{
		Format: config.JSON,
		Level:  slog.LevelError,
	}
	logger, _ := Logger(cfg)
	slog.SetDefault(logger)
}

func LogBuildInformation() {
	if build, ok := debug.ReadBuildInfo(); ok {
		slog.Info(
			"build info",
			"service", internal.Project, "version", internal.Version, "go_version", build.GoVersion,
		)
	}
}

func Logger(cfg config.Log) (*slog.Logger, error) {
	options := &slog.HandlerOptions{
		AddSource: cfg.Source,
		Level:     cfg.Level,
	}
	var handler slog.Handler
	switch cfg.Format {
	case config.JSON:
		handler = slog.NewJSONHandler(os.Stdout, options)
	case config.Text:
		handler = slog.NewTextHandler(os.Stdout, options)
	default:
		return nil, fmt.Errorf("unhandled log format: %q", cfg.Format)
	}
	return slog.New(handler), nil
}

func SetDefault(logger *slog.Logger) {
	slog.SetDefault(logger)
}
