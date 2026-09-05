package kernel

import (
	"context"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// Store is the persistence contract. It is implemented by internal/store/sqlite
// today and can be reimplemented on Postgres later without touching callers.
type Store interface {
	EnsureOwner(ctx context.Context, owner domain.OwnerID) error
	UpsertAsset(ctx context.Context, a domain.Asset) error
	ListAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error)
	// IndexPalette stores an asset's extracted palette: the palette and dominant
	// color as extracted-layer annotations, plus the searchable color index. The
	// asset is addressed by its natural key so the canonical row id is resolved
	// even when a re-scan generated a fresh id that was discarded on upsert.
	IndexPalette(ctx context.Context, owner domain.OwnerID, provider, path string, pal color.Palette) error
	// SearchByColor returns assets whose palette contains a swatch within tol
	// (CIEDE2000) of target, nearest first, capped at limit.
	SearchByColor(ctx context.Context, owner domain.OwnerID, target color.Lab, tol float64, limit int) ([]domain.ColorMatch, error)
	Close() error
}
