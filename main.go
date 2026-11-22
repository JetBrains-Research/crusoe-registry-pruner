package main

import (
	"log/slog"
	"os"

	"crusoe-registry-pruner/internal/logging"
	"crusoe-registry-pruner/internal/pruner"
)

func main() {
	logging.InitializeDefault()
	if err := pruner.Prune(); err != nil {
		slog.Error("pruning failed", "error", err)
		os.Exit(1)
	}
}
