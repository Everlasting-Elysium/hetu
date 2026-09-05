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

	// AIAddr is the base URL of the Python AI sidecar (see internal/ai). When
	// non-empty the indexer enqueues AI tagging jobs after an asset is indexed;
	// set it empty to disable AI orchestration entirely. Local-first by default.
	AIAddr string `env:"HETU_AI_ADDR" envDefault:"http://localhost:8091"`

	// NASProvider selects which registered storage provider the NAS plugin
	// browses ("local" or "rclone"). Defaults to the local filesystem.
	NASProvider string `env:"HETU_NAS_PROVIDER" envDefault:"local"`

	// Rclone RC daemon configuration. When RcloneAddr is non-empty the rclone
	// StorageProvider is registered. The daemon must run with --rc-serve so
	// that file content is available via HTTP GET on the same address.
	RcloneAddr   string `env:"HETU_RCLONE_ADDR"`
	RcloneRemote string `env:"HETU_RCLONE_REMOTE" envDefault:"remote:"`
	RcloneUser   string `env:"HETU_RCLONE_USER"`
	RclonePass   string `env:"HETU_RCLONE_PASS"`

	// BlenderAddr is the host:port of the Blender headless sidecar used to
	// render 3D-model thumbnails. Empty disables 3D thumbnailing: models are
	// still indexed, they just have no thumbnail (graceful degradation).
	BlenderAddr string `env:"HETU_BLENDER_ADDR"`
}

// Load parses the environment into a Config.
func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}
