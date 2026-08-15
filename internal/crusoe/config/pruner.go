package config

import (
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
)

type Log struct {
	Format Format     `env:"FORMAT" envDefault:"json"`
	Level  slog.Level `env:"LEVEL" envDefault:"info"`
	Source bool       `env:"SOURCE" envDefault:"false"`
}

type Pruner struct {
	Timeout         time.Duration `env:"TIMEOUT" envDefault:"30m"`
	MaxAge          time.Duration `env:"MAX_AGE" envDefault:"720h"`
	AgeFrom         AgeFrom       `env:"AGE_FROM" envDefault:"pushed"`
	TagState        TagState      `env:"TAG_STATE" envDefault:"any"`
	KeepTagPrefixes TagPrefixes   `env:"KEEP_TAG_PREFIXES" envDefault:""`
	DeleteImages    bool          `env:"DELETE_IMAGES" envDefault:"false"`
	DryRun          bool          `env:"DRY_RUN" envDefault:"false"`
	Log             Log           `envPrefix:"LOG_"`
}

type Crusoe struct {
	ProjectId ID     `env:"PROJECT_ID,required,notEmpty"`
	AccessKey Secret `env:"ACCESS_KEY,required,notEmpty"`
	SecretKey Secret `env:"SECRET_KEY,required,notEmpty"`
	Pruner    Pruner `envPrefix:"PRUNER_"`
}

func Load() (*Crusoe, error) {
	cfg := &Crusoe{}
	opts := env.Options{Prefix: "CRUSOE_"}
	if err := env.ParseWithOptions(cfg, opts); err != nil {
		return nil, err
	}
	return cfg, nil
}
