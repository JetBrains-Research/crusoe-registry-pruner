package main

import (
	"crusoe-registry-pruner/internal/crusoe"
	"crusoe-registry-pruner/internal/crusoe/logging"
	"log/slog"
	"os"
)

func main() {
	logging.InitializeDefaultLogger()
	if err := crusoe.PruneCcr(); err != nil {
		slog.Error("pruning failed", "error", err)
		os.Exit(1)
	}
}
