package logging

import (
	"crusoe-registry-pruner/internal/crusoe/config"
	"fmt"
	"log/slog"
	"os"
)

func InitializeDefaultLogger() {
	cfg := config.Log{
		Format: config.JSON,
		Level:  slog.LevelError,
	}
	logger, _ := Logger(cfg)
	slog.SetDefault(logger)
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
