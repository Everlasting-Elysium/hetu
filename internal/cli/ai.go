package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/Everlasting-Elysium/hetu/internal/ai"
	"github.com/Everlasting-Elysium/hetu/internal/app"
)

// newAICmd is the `hetu ai` command group for maintaining the AI metadata layer.
func newAICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ai",
		Short: "Maintain the AI metadata layer (auto-tagging)",
	}
	cmd.AddCommand(newAIRetagCmd(), newAIClearCmd())
	return cmd
}

func newAIRetagCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retag",
		Short: "Re-run AI tagging over the whole library, persisting to the ai layer",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAIRetag(cmd.Context())
		},
	}
}

func newAIClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Remove all ai-layer tags and annotations (manual data is untouched)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAIClear(cmd.Context())
		},
	}
}

func runAIRetag(ctx context.Context) error {
	cfg, log, err := load()
	if err != nil {
		return err
	}
	if cfg.AIAddr == "" {
		return fmt.Errorf("ai retag: sidecar not configured (set HETU_AI_ADDR)")
	}
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()

	client := ai.New(ai.Config{BaseURL: cfg.AIAddr, Logger: log})
	tagged, skipped, err := ai.Retag(ctx, client, a.Store(), a.Owner, log)
	if err != nil {
		return fmt.Errorf("ai retag: %w", err)
	}
	log.InfoContext(ctx, "ai retag done",
		slog.Int("tagged", tagged), slog.Int("skipped", skipped))
	return nil
}

func runAIClear(ctx context.Context) error {
	cfg, log, err := load()
	if err != nil {
		return err
	}
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()

	if err := a.Store().ClearAILayer(ctx, a.Owner); err != nil {
		return fmt.Errorf("ai clear: %w", err)
	}
	log.InfoContext(ctx, "ai layer cleared", slog.String("owner", a.Owner.String()))
	return nil
}
