package utils

import (
	"strconv"
)

func ParseSizeBytes(size string) int64 {
	if size == "" {
		return int64(0)
	} else if value, err := strconv.ParseInt(size, 10, 64); err != nil {
		return int64(0)
	} else {
		return max(0, value)
	}
}
