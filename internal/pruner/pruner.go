package pruner

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"math"
	"os/signal"
	"syscall"
	"time"

	"crusoe-registry-pruner/internal/config"
	"crusoe-registry-pruner/internal/crusoe"
	"crusoe-registry-pruner/internal/logging"
)

var (
	errCleanupFailures = errors.New("cleanup had failures")
	errTimeoutExceeded = errors.New("timeout exceeded")
)

func Prune() error {
	start := time.Now()
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := logging.Logger(cfg.Pruner.Log)
	if err != nil {
		return err
	}
	logging.SetDefault(logger)
	logging.LogBuildInformation()

	summary := Summary{}
	defer func() {
		summary.Duration = time.Since(start)
		slog.Info("finished cleanup", "summary", summary)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	timeout := cmp.Or(cfg.Pruner.Timeout, time.Duration(math.MaxInt64))
	ctx, cancel := context.WithTimeoutCause(ctx, timeout, errTimeoutExceeded)
	defer cancel()

	client, err := crusoe.NewClient(cfg, logger)
	if err != nil {
		return err
	}

	if cfg.Pruner.DryRun {
		slog.Info("dry run mode enabled")
	}

	slog.Info("starting cleanup", "project", cfg.ProjectID)
	repositories, err := client.GetRepositories(ctx)
	if err != nil {
		return err
	}

	for _, repository := range repositories {
		if err := context.Cause(ctx); err != nil {
			return err
		}

		summary.Analyzed.Repositories++
		images, err := client.GetImages(ctx, repository)
		if err != nil {
			slog.Warn("skipping repository", "error", err)
			summary.Failed.Listings++
			continue
		}

		for _, image := range images {
			if err := context.Cause(ctx); err != nil {
				return err
			}

			summary.Analyzed.Images++
			manifests, err := client.GetManifests(ctx, repository, image)
			if err != nil {
				slog.Warn("skipping image", "error", err)
				summary.Failed.Listings++
				continue
			}

			deleted := 0
			for _, manifest := range manifests {
				if err := context.Cause(ctx); err != nil {
					return err
				}

				summary.Analyzed.Manifests++
				sizeBytes := parseSizeBytes(manifest.SizeBytes)
				if shouldPrune(cfg.Pruner, manifest, start) {
					err := client.DeleteManifest(ctx, repository, image, manifest)
					if err != nil {
						slog.Warn("failed to delete manifest", "error", err)
						summary.Failed.Deletions++
					} else {
						summary.Bytes += sizeBytes
						summary.Deleted.Manifests++
						deleted++
					}
				}
			}

			if cfg.Pruner.DeleteImages && len(manifests) == deleted {
				err := client.DeleteImage(ctx, repository, image)
				if err != nil {
					slog.Warn("failed to delete image", "error", err)
					summary.Failed.Deletions++
				} else {
					summary.Deleted.Images++
				}
			}
		}
	}

	if summary.Failed.Total() > 0 {
		return errCleanupFailures
	}

	return nil
}
