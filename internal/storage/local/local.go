// Package local implements kernel.StorageProvider over the local filesystem,
// rooted at a configurable library directory. Paths are cleaned so they cannot
// escape the root. rclone/AList providers (network drives as S3/VFS) come next.
package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// ProviderName is the registry key for the local filesystem provider.
const ProviderName = "local"

// Provider serves files under Root.
type Provider struct {
	root string
}

var (
	_ kernel.StorageProvider = (*Provider)(nil)
	_ kernel.StorageWriter   = (*Provider)(nil)
)

// New returns a local provider rooted at root.
func New(root string) *Provider { return &Provider{root: root} }

// Name returns the provider registry key.
func (p *Provider) Name() string { return ProviderName }

// resolve maps a provider-relative path to an absolute path inside root,
// stripping any "../" so callers cannot escape the root.
func (p *Provider) resolve(path string) string {
	return filepath.Join(p.root, filepath.Join("/", path))
}

// List returns the entries directly under path.
func (p *Provider) List(_ context.Context, path string) ([]domain.Entry, error) {
	dirents, err := os.ReadDir(p.resolve(path))
	if err != nil {
		return nil, fmt.Errorf("list %q: %w", path, err)
	}
	out := make([]domain.Entry, 0, len(dirents))
	for _, d := range dirents {
		info, err := d.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", d.Name(), err)
		}
		out = append(out, domain.Entry{
			Name:    d.Name(),
			Path:    filepath.Join(path, d.Name()),
			IsDir:   d.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	return out, nil
}

// Open opens the file at path for reading.
func (p *Provider) Open(_ context.Context, path string) (io.ReadSeekCloser, error) {
	f, err := os.Open(p.resolve(path))
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	return f, nil
}

// Stat returns metadata for the object at path.
func (p *Provider) Stat(_ context.Context, path string) (domain.FileInfo, error) {
	info, err := os.Stat(p.resolve(path))
	if err != nil {
		return domain.FileInfo{}, fmt.Errorf("stat %q: %w", path, err)
	}
	return domain.FileInfo{Size: info.Size(), IsDir: info.IsDir(), ModTime: info.ModTime()}, nil
}

// Write copies r to path inside root, creating parent directories. It implements
// kernel.StorageWriter for managed writes (asset version copies).
func (p *Provider) Write(_ context.Context, path string, r io.Reader) (int64, error) {
	abs := p.resolve(path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return 0, fmt.Errorf("create dir for %q: %w", path, err)
	}
	f, err := os.Create(abs)
	if err != nil {
		return 0, fmt.Errorf("create %q: %w", path, err)
	}
	n, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(abs)
		return 0, fmt.Errorf("write %q: %w", path, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(abs)
		return 0, fmt.Errorf("close %q: %w", path, closeErr)
	}
	return n, nil
}

// Remove deletes the file at path. A missing file is not an error so version
// cleanup is idempotent. It implements kernel.StorageWriter.
func (p *Provider) Remove(_ context.Context, path string) error {
	if err := os.Remove(p.resolve(path)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %q: %w", path, err)
	}
	return nil
}
