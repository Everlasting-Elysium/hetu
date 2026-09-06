package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

var _ kernel.WritableProvider = (*Provider)(nil)

// Write streams r into path under the library root, creating parent dirs. It
// writes to a temporary sibling then hard-links it into place, so a reader never
// observes a partially written file (atomic visibility on the same filesystem)
// AND an existing destination is never overwritten: os.Link fails with EEXIST,
// which the import service surfaces as a loud failure rather than silent data
// loss (the caller pre-selects a free name; this closes the residual TOCTOU
// window under concurrent same-name writes).
func (p *Provider) Write(_ context.Context, path string, r io.Reader) (int64, error) {
	dst := p.resolve(path)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, fmt.Errorf("create dir for %q: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".hetu-import-*")
	if err != nil {
		return 0, fmt.Errorf("create temp for %q: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	n, copyErr := io.Copy(tmp, r)
	closeErr := tmp.Close()
	if copyErr != nil {
		return 0, fmt.Errorf("write %q: %w", path, copyErr)
	}
	if closeErr != nil {
		return 0, fmt.Errorf("close temp for %q: %w", path, closeErr)
	}
	if err := os.Link(tmpName, dst); err != nil {
		return 0, fmt.Errorf("place %q (destination may already exist): %w", path, err)
	}
	return n, nil
}

// Remove deletes the object at path (used to roll back a failed import write).
func (p *Provider) Remove(_ context.Context, path string) error {
	if err := os.Remove(p.resolve(path)); err != nil {
		return fmt.Errorf("remove %q: %w", path, err)
	}
	return nil
}

// Rename moves oldPath to newPath within the library root, creating the
// destination's parent dir first.
func (p *Provider) Rename(_ context.Context, oldPath, newPath string) error {
	dst := p.resolve(newPath)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dir for %q: %w", newPath, err)
	}
	if err := os.Rename(p.resolve(oldPath), dst); err != nil {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, err)
	}
	return nil
}
