package config

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

type AgeFrom string

const (
	Pushed   AgeFrom = "pushed"
	Pulled   AgeFrom = "pulled"
	Activity AgeFrom = "activity"
)

func (ageFrom *AgeFrom) UnmarshalText(text []byte) error {
	normalized := bytes.ToLower(text)
	switch value := AgeFrom(normalized); value {
	case Pushed, Pulled, Activity:
		*ageFrom = value
		return nil
	default:
		return fmt.Errorf("invalid age from %q: want %q, %q or %q", value, Pushed, Pulled, Activity)
	}
}

type Format string

const (
	JSON Format = "json"
	Text Format = "text"
)

func (format *Format) UnmarshalText(text []byte) error {
	normalized := bytes.ToLower(text)
	switch value := Format(normalized); value {
	case JSON, Text:
		*format = value
		return nil
	default:
		return fmt.Errorf("invalid log format %q: want %q or %q", value, JSON, Text)
	}
}

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
	raw := strings.Split(string(text), ",")
	for idx := range raw {
		raw[idx] = strings.TrimSpace(raw[idx])
	}

	raw = slices.DeleteFunc(raw, func(item string) bool {
		return item == ""
	})

	slices.Sort(raw)
	*tagPrefixes = slices.Compact(raw)
	return nil
}
