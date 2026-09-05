package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/Everlasting-Elysium/hetu/internal/app"
	"github.com/Everlasting-Elysium/hetu/internal/index"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
)

func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan [path]",
		Short: "Index assets under the library path (relative to HETU_LIBRARY_DIR)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			return runScan(cmd.Context(), path)
		},
	}
}

func runScan(ctx context.Context, path string) error {
	cfg, log, err := load()
	if err != nil {
		return err
	}
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()

	res, err := index.New(a.Kernel, a.Owner).Scan(ctx, local.ProviderName, path)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	log.InfoContext(ctx, "scan complete",
		slog.Int("indexed", res.Indexed), slog.Int("skipped", res.Skipped))
	return nil
}
