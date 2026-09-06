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

// StorageWriter is an optional StorageProvider capability for backends that can
// accept managed writes (currently the local provider). Asset versioning copies
// uploaded revisions into the library through it; callers type-assert and return
// domain.ErrUnsupported when a provider does not implement it — the base
// read-only StorageProvider contract stays unchanged. This mirrors the optional
// AssetHandler capabilities (PaletteExtractor, PHashExtractor, MetadataExtractor).
type StorageWriter interface {
	// Write copies all bytes from r to path (creating parent directories),
	// returning the number of bytes written.
	Write(ctx context.Context, path string, r io.Reader) (int64, error)
	// Remove deletes the file at path. Removing a non-existent path is not an
	// error, so cleanup is idempotent.
	Remove(ctx context.Context, path string) error
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
