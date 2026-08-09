package crusoe

import (
	"log/slog"
	"time"
)

type Resources struct {
	Repositories int
	Images       int
	Manifests    int
}

func (deletions Resources) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("repositories", deletions.Repositories),
		slog.Int("images", deletions.Images),
		slog.Int("manifests", deletions.Manifests),
	)
}

type Failures struct {
	Listings  int
	Deletions int
}

func (failures Failures) Total() int {
	return failures.Listings + failures.Deletions
}

func (failures Failures) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("listings", failures.Listings),
		slog.Int("deletions", failures.Deletions),
	)
}

type Summary struct {
	Analyzed Resources
	Deleted  Resources
	Failed   Failures
	Duration time.Duration
	Bytes    int64
}

func (summary Summary) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("analyzed", summary.Analyzed),
		slog.Any("deleted", summary.Deleted),
		slog.Any("failed", summary.Failed),
		slog.Duration("duration", summary.Duration),
		slog.Int64("bytes", summary.Bytes),
	)
}
