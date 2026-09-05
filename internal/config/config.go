// Package config parses hetu's runtime configuration from the environment.
// v0 is env-based (12-factor, Docker-friendly); a YAML file loader is on the
// roadmap. The plugin enable-list lives in HETU_PLUGINS ("dam,nas").
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config is the full runtime configuration.
type Config struct {
	Addr       string   `env:"HETU_ADDR" envDefault:":8080"`
	DataDir    string   `env:"HETU_DATA_DIR" envDefault:"./data"`
	DBPath     string   `env:"HETU_DB_PATH" envDefault:"./data/hetu.db"`
	LibraryDir string   `env:"HETU_LIBRARY_DIR" envDefault:"."`
	Plugins    []string `env:"HETU_PLUGINS" envSeparator:"," envDefault:"dam,nas"`
	LogLevel   string   `env:"HETU_LOG_LEVEL" envDefault:"info"`
	// Owner is the single-user owner id until multi-user lands.
	Owner string `env:"HETU_OWNER" envDefault:"default"`
}

// Load parses the environment into a Config.
func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}
