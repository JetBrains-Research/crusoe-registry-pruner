package config

import (
	"bytes"
	"fmt"
)

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
