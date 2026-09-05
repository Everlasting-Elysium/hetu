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
