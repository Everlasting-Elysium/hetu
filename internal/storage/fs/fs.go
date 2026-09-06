// Package fs implements a read-only kernel.StorageProvider that addresses files
// by their absolute filesystem path. It exists so assets migrated "in place"
// from an external library (Eagle/Billfish, outside HETU_LIBRARY_DIR) can be
// indexed where they live and still served after a restart: the provider is
// registered under a constant name at startup, so provider="fs" rows always
// resolve without persisting per-import provider registrations.
//
// It is intentionally NOT a NAS browse target (never set HETU_NAS_PROVIDER=fs):
// it only opens paths hetu itself indexed. Thumbnails are written to the data
// dir at index time and served independently of this provider.
package fs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// ProviderName is the registry key for the absolute-path filesystem provider.
const ProviderName = "fs"

// Provider serves files addressed by absolute path.
type Provider struct{}

var _ kernel.StorageProvider = (*Provider)(nil)

// New returns an absolute-path filesystem provider.
func New() *Provider { return &Provider{} }

// Name returns the provider registry key.
func (p *Provider) Name() string { return ProviderName }

// resolve returns the cleaned absolute path. A relative path is anchored at the
// filesystem root so it can never resolve against the process working dir.
func (p *Provider) resolve(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(string(filepath.Separator) + path)
}

// List returns the entries directly under path (absolute). Provided for
// interface completeness; imports index single files via Indexer.IndexFile and
// this provider is never used for NAS browsing.
func (p *Provider) List(_ context.Context, path string) ([]domain.Entry, error) {
	abs := p.resolve(path)
	dirents, err := os.ReadDir(abs)
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
			Path:    filepath.Join(abs, d.Name()),
			IsDir:   d.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	return out, nil
}

// Open opens the file at the absolute path for reading.
func (p *Provider) Open(_ context.Context, path string) (io.ReadSeekCloser, error) {
	f, err := os.Open(p.resolve(path))
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	return f, nil
}

// Stat returns metadata for the object at the absolute path.
func (p *Provider) Stat(_ context.Context, path string) (domain.FileInfo, error) {
	info, err := os.Stat(p.resolve(path))
	if err != nil {
		return domain.FileInfo{}, fmt.Errorf("stat %q: %w", path, err)
	}
	return domain.FileInfo{Size: info.Size(), IsDir: info.IsDir(), ModTime: info.ModTime()}, nil
}
