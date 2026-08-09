package policy

import (
	"crusoe-registry-pruner/internal/crusoe/config"
	"slices"
	"strings"
	"time"

	"github.com/crusoecloud/client-go/swagger/v1alpha5"
)

func ShouldPrune(
	pruner config.Pruner,
	manifest swagger.Manifest,
	now time.Time,
) bool {
	for _, tag := range manifest.Tags {
		for _, prefix := range pruner.KeepTagPrefixes {
			if strings.HasPrefix(tag, prefix) {
				return false
			}
		}
	}

	switch pruner.TagState {
	case config.Tagged:
		if len(manifest.Tags) == 0 {
			return false
		}
	case config.Untagged:
		if len(manifest.Tags) > 0 {
			return false
		}
	}

	if pruner.MaxAge == 0 {
		return false
	}

	reference := time.Time{}
	switch pruner.AgeFrom {
	case config.Pushed:
		reference = manifest.PushedAt
	case config.Pulled:
		reference = manifest.PulledAt
	case config.Activity:
		reference = slices.MaxFunc([]time.Time{manifest.PulledAt, manifest.PushedAt}, time.Time.Compare)
	default:
		return false
	}
	if reference.IsZero() {
		return false // no timestamp to judge age against
	}
	return now.After(reference.Add(pruner.MaxAge))
}
