// Package cli is the cobra command tree for the hetu binary.
package cli

import (
	"context"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/Everlasting-Elysium/hetu/internal/config"
	"github.com/Everlasting-Elysium/hetu/internal/obs"
)

// Execute runs the root command with ctx (cancelled on SIGINT/SIGTERM).
func Execute(ctx context.Context) error {
	root := &cobra.Command{
		Use:           "hetu",
		Short:         "hetu — self-hosted AI-native NAS + asset manager",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newServeCmd(), newScanCmd())
	return root.ExecuteContext(ctx)
}

// load parses config and builds the logger, shared by subcommands.
func load() (config.Config, *slog.Logger, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, obs.NewLogger(cfg.LogLevel), nil
}
