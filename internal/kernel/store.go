package kernel

import (
	"context"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// Store is the persistence contract. It is implemented by internal/store/sqlite
// today and can be reimplemented on Postgres later without touching callers.
type Store interface {
	EnsureOwner(ctx context.Context, owner domain.OwnerID) error
	UpsertAsset(ctx context.Context, a domain.Asset) error
	ListAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error)
	Close() error
}
