package crusoe

import (
	"context"
	"crusoe-registry-pruner/internal/crusoe/config"
	"crusoe-registry-pruner/internal/crusoe/logging"
	"crusoe-registry-pruner/internal/crusoe/policy"
	"crusoe-registry-pruner/internal/crusoe/utils"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
)

func PruneCcr() error {
	start := time.Now()
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := logging.Logger(cfg.Pruner.Log)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

	summary := Summary{}
	defer func() {
		summary.Duration = time.Since(start)
		slog.Info("finished cleanup", "summary", summary)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeoutCause(ctx, cfg.Pruner.Timeout, errors.New("timeout exceeded"))
	defer cancel()

	client, err := NewClient(cfg, logger)
	if err != nil {
		return err
	}

	if build, ok := debug.ReadBuildInfo(); ok {
		slog.Info("build info", "version", utils.Version, "go_version", build.GoVersion)
	}

	if cfg.Pruner.DryRun {
		slog.Info("dry run mode enabled")
	}

	slog.Info("starting cleanup", "project", cfg.ProjectId.String())
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
				sizeBytes := utils.ParseSizeBytes(manifest.SizeBytes)
				if policy.ShouldPrune(cfg.Pruner, manifest, start) {
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
		return errors.New("cleanup had failures")
	}

	return nil
}
