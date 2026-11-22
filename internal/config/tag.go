package config

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

type TagState string

const (
	Any      TagState = "any"
	Tagged   TagState = "tagged"
	Untagged TagState = "untagged"
)

func (tagState *TagState) UnmarshalText(text []byte) error {
	normalized := bytes.ToLower(text)
	switch value := TagState(normalized); value {
	case Any, Tagged, Untagged:
		*tagState = value
		return nil
	default:
		return fmt.Errorf("invalid tag state %q: want %q, %q or %q", value, Any, Tagged, Untagged)
	}
}

type TagPrefixes []string

func (tagPrefixes *TagPrefixes) UnmarshalText(text []byte) error {
	prefixes := TagPrefixes{}
	for raw := range strings.SplitSeq(string(text), ",") {
		if prefix := strings.TrimSpace(raw); prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}

	slices.Sort(prefixes)
	*tagPrefixes = slices.Compact(prefixes)
	return nil
}
