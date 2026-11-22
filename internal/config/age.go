package config

import (
	"bytes"
	"fmt"
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
