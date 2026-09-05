// Package app is the composition root: it wires config into a kernel with
// storage providers, asset handlers, and the enabled capability plugins.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Everlasting-Elysium/hetu/internal/ai"
	"github.com/Everlasting-Elysium/hetu/internal/asset/document"
	"github.com/Everlasting-Elysium/hetu/internal/asset/image"
	"github.com/Everlasting-Elysium/hetu/internal/asset/model3d"
	"github.com/Everlasting-Elysium/hetu/internal/asset/video"
	"github.com/Everlasting-Elysium/hetu/internal/config"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/dam"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/nas"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/storage/rclone"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// App is the wired application: kernel, enabled plugins, and owning identity.
type App struct {
	Cfg     config.Config
	Kernel  *kernel.Kernel
	Plugins []kernel.Plugin
	Owner   domain.OwnerID

	store *store.SQLite
}

// New builds the App from cfg. The caller must call Close when done.
func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	owner, err := domain.NewOwnerID(cfg.Owner)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", cfg.DataDir, err)
	}
	if dir := filepath.Dir(cfg.DBPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir %q: %w", dir, err)
		}
	}

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, err
	}
	k := kernel.New(kernel.Deps{
		Log:         log,
		Store:       st,
		ThumbDir:    filepath.Join(cfg.DataDir, "thumbnails"),
		JobBuffer:   64,
		BlenderAddr: cfg.BlenderAddr,
	})
	k.Storage.Register(local.New(cfg.LibraryDir))
	if cfg.RcloneAddr != "" {
		k.Storage.Register(rclone.New(cfg.RcloneAddr, cfg.RcloneRemote, cfg.RcloneUser, cfg.RclonePass))
		log.Info("registered rclone storage provider", slog.String("addr", cfg.RcloneAddr), slog.String("remote", cfg.RcloneRemote))
	}
	k.Assets.Register(image.New())
	k.Assets.Register(model3d.New(cfg.BlenderAddr))
	k.Assets.Register(video.New(log))
	k.Assets.Register(document.New(log))

	if _, ok := k.Storage.Get(cfg.NASProvider); !ok {
		_ = st.Close()
		return nil, fmt.Errorf("nas provider %q not registered (set HETU_RCLONE_ADDR to enable rclone)", cfg.NASProvider)
	}
	plugins, err := buildPlugins(cfg.Plugins, owner, cfg.NASProvider)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	for _, p := range plugins {
		if err := p.Init(ctx, k); err != nil {
			_ = st.Close()
			return nil, fmt.Errorf("init plugin %q: %w", p.Name(), err)
		}
	}
	// Wire AI orchestration: index → enqueue ai_tag job → sidecar client →
	// persist to the ai layer (store). An empty HETU_AI_ADDR disables it. Jobs
	// run on the server's JobQueue workers.
	if cfg.AIAddr != "" {
		ai.Subscribe(k, ai.New(ai.Config{BaseURL: cfg.AIAddr, Logger: log}), st)
		log.Info("registered AI orchestration", slog.String("addr", cfg.AIAddr))
	}
	return &App{Cfg: cfg, Kernel: k, Plugins: plugins, Owner: owner, store: st}, nil
}

// Close releases resources (the database).
func (a *App) Close() error { return a.store.Close() }

// Store returns the underlying store for maintenance commands (e.g. the AI
// retag/clear CLI, which needs ai-layer methods not on the kernel.Store contract).
func (a *App) Store() *store.SQLite { return a.store }

func buildPlugins(names []string, owner domain.OwnerID, nasProvider string) ([]kernel.Plugin, error) {
	plugins := make([]kernel.Plugin, 0, len(names))
	for _, name := range names {
		switch name {
		case dam.Name:
			plugins = append(plugins, dam.New(owner))
		case nas.Name:
			plugins = append(plugins, nas.New(owner, nasProvider))
		default:
			return nil, fmt.Errorf("unknown plugin %q", name)
		}
	}
	return plugins, nil
}
