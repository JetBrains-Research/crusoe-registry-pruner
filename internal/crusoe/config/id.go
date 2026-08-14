package config

import (
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

type ID uuid.UUID

func (id ID) GoString() string {
	return fmt.Sprintf("config.ID(%q)", id.String())
}

func (id ID) LogValue() slog.Value {
	return slog.StringValue(id.String())
}

func (id ID) String() string {
	return uuid.UUID(id).String()
}

func (id *ID) UnmarshalText(text []byte) error {
	value, err := uuid.ParseBytes(text)
	if err != nil {
		return err
	}
	if value == uuid.Nil {
		return fmt.Errorf("invalid id: %q", text)
	}

	*id = ID(value)
	return nil
}
