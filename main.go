package main

import (
	"log/slog"
	"os"

	"crusoe-registry-pruner/internal/crusoe"
	"crusoe-registry-pruner/internal/crusoe/logging"
)

func main() {
	logging.InitializeDefaultLogger()
	if err := crusoe.PruneCcr(); err != nil {
		slog.Error("pruning failed", "error", err)
		os.Exit(1)
	}
}
