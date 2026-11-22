package pruner

import (
	"strconv"
)

func parseSizeBytes(size string) int64 {
	if size == "" {
		return int64(0)
	}
	value, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return int64(0)
	}
	return max(0, value)
}
