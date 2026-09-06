package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/Everlasting-Elysium/hetu/internal/app"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/importers"
	"github.com/Everlasting-Elysium/hetu/internal/importers/billfish"
	"github.com/Everlasting-Elysium/hetu/internal/importers/eagle"
)

func newImportCmd() *cobra.Command {
	var mode, conflict string
	cmd := &cobra.Command{
		Use:   "import <eagle|billfish> <library-path>",
		Short: "Migrate an Eagle or Billfish library into hetu (read-only on the source)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd.Context(), args[0], args[1], mode, conflict)
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "index", "placement mode: index (in place) or copy (into library)")
	cmd.Flags().StringVar(&conflict, "conflict", "keep-both", "duplicate policy: keep-both or skip (by content hash)")
	return cmd
}

func runImport(ctx context.Context, source, path, mode, conflict string) error {
	cfg, log, err := load()
	if err != nil {
		return err
	}
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()

	src, err := openSource(source, path)
	if err != nil {
		return err
	}
	svc := importers.New(a.Kernel, a.Owner)
	res, err := svc.ImportSource(ctx, src, importers.Options{
		Mode: importers.Mode(mode), Conflict: importers.Conflict(conflict),
	}, domain.JobID{})
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	log.InfoContext(ctx, "migration complete",
		slog.String("source", source),
		slog.Int("total", res.Total), slog.Int("imported", res.Imported),
		slog.Int("skipped", res.Skipped), slog.Int("failed", res.Failed))
	return nil
}

// openSource opens a migration source library read-only.
func openSource(source, path string) (importers.Source, error) {
	switch source {
	case string(importers.KindEagle):
		return eagle.Open(path)
	case string(importers.KindBillfish):
		return billfish.Open(path)
	default:
		return nil, fmt.Errorf("unknown source %q (want eagle|billfish)", source)
	}
}
