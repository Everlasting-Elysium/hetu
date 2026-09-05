package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/app"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the hetu HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context())
		},
	}
}

func runServe(ctx context.Context) error {
	cfg, log, err := load()
	if err != nil {
		return err
	}
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()

	a.Kernel.Jobs.Start(ctx, 2)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewRouter(a.Kernel, a.Plugins),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.InfoContext(ctx, "hetu serving",
		slog.String("addr", cfg.Addr), slog.Any("plugins", cfg.Plugins))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
