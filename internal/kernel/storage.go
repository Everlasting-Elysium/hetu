package kernel

import (
	"context"
	"io"
	"sync"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// StorageProvider abstracts a storage backend. v0 ships a local filesystem
// provider; rclone/AList providers (network drives exposed as S3/VFS) come next.
type StorageProvider interface {
	Name() string
	List(ctx context.Context, path string) ([]domain.Entry, error)
	Open(ctx context.Context, path string) (io.ReadSeekCloser, error)
	Stat(ctx context.Context, path string) (domain.FileInfo, error)
}

// WritableProvider is an optional capability for providers that can accept
// writes, discovered via type assertion (mirroring the AssetHandler optional
// interfaces). Only the local provider implements it today; it backs the
// import API's copy/move modes. Write must be atomic in its visibility (write
// to a temp path then rename) so a partially written file is never observable.
type WritableProvider interface {
	StorageProvider
	// Write streams r into path (creating parent dirs) and returns bytes
	// written. It places the file atomically and MUST fail rather than
	// overwrite an existing destination, so a concurrent same-path write is a
	// loud error, not silent data loss.
	Write(ctx context.Context, path string, r io.Reader) (int64, error)
	// Remove deletes the object at path (used to roll back a failed import).
	Remove(ctx context.Context, path string) error
	// Rename moves oldPath to newPath within the provider.
	Rename(ctx context.Context, oldPath, newPath string) error
}

// StorageRegistry holds the enabled storage providers by name.
type StorageRegistry struct {
	mu        sync.RWMutex
	providers map[string]StorageProvider
}

// NewStorageRegistry returns an empty registry.
func NewStorageRegistry() *StorageRegistry {
	return &StorageRegistry{providers: make(map[string]StorageProvider)}
}

// Register adds a provider, keyed by its Name.
func (r *StorageRegistry) Register(p StorageProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Get returns the provider registered under name.
func (r *StorageRegistry) Get(name string) (StorageProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}
