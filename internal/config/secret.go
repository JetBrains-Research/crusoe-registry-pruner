package config

import "log/slog"

type Secret string

func (secret Secret) GoString() string {
	return "\"***\""
}

func (secret Secret) LogValue() slog.Value {
	return slog.StringValue("***")
}

func (secret Secret) String() string {
	return "***"
}
