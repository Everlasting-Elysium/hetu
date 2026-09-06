package importers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/index"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/storage/fs"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
)

func noopCleanup(context.Context) error { return nil }

// canonicalPath resolves absPath to a cleaned, symlink-free absolute path so the
// (owner, fs, storage_path) natural key is stable across equivalent spellings
// (trailing slashes, symlinks) and re-imports dedupe correctly.
func canonicalPath(absPath string) (string, error) {
	if absPath == "" {
		return "", fmt.Errorf("import: empty source path")
	}
	abs, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve abs %q: %w", absPath, err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved // best-effort; keep abs if the path is not yet resolvable
	}
	return filepath.Clean(abs), nil
}

// place positions item's file for indexing per opt.Mode and returns the target
// provider name, the entry to index, and a cleanup that rolls back a copy/move
// destination so an index failure never leaves an orphan in the library.
func (s *Service) place(ctx context.Context, canonical string, item ImportItem, opt Options) (string, domain.Entry, func(context.Context) error, error) {
	switch opt.mode() {
	case ModeIndex:
		prov, ok := s.k.Storage.Get(fs.ProviderName)
		if !ok {
			return "", domain.Entry{}, noopCleanup, fmt.Errorf("place: provider %q: %w", fs.ProviderName, domain.ErrNotFound)
		}
		entry, err := index.StatEntry(ctx, prov, canonical)
		if err != nil {
			return "", domain.Entry{}, noopCleanup, err
		}
		applyItemTime(&entry, item)
		return fs.ProviderName, entry, noopCleanup, nil
	case ModeCopy, ModeMove:
		return s.placeIntoLibrary(ctx, canonical, item, opt)
	default:
		return "", domain.Entry{}, noopCleanup, fmt.Errorf("import: unknown mode %q", opt.Mode)
	}
}

// placeIntoLibrary copies canonical into the local library under
// opt.DestSubdir/<name> (uniquified to avoid clobbering) and returns the local
// entry plus a cleanup that removes the copy. Used by copy and move alike; move
// deletes the source only after the whole import succeeds (see ImportItem).
func (s *Service) placeIntoLibrary(ctx context.Context, canonical string, item ImportItem, opt Options) (string, domain.Entry, func(context.Context) error, error) {
	src, ok := s.k.Storage.Get(fs.ProviderName)
	if !ok {
		return "", domain.Entry{}, noopCleanup, fmt.Errorf("place: provider %q: %w", fs.ProviderName, domain.ErrNotFound)
	}
	dstProv, ok := s.k.Storage.Get(local.ProviderName)
	if !ok {
		return "", domain.Entry{}, noopCleanup, fmt.Errorf("place: provider %q: %w", local.ProviderName, domain.ErrNotFound)
	}
	writable, ok := dstProv.(kernel.WritableProvider)
	if !ok {
		return "", domain.Entry{}, noopCleanup, fmt.Errorf("place: provider %q is not writable", local.ProviderName)
	}
	name := item.Name
	if name == "" {
		name = filepath.Base(canonical)
	}
	relPath := s.uniqueDest(ctx, dstProv, filepath.Join(opt.DestSubdir, name))

	rc, err := src.Open(ctx, canonical)
	if err != nil {
		return "", domain.Entry{}, noopCleanup, err
	}
	written, writeErr := writable.Write(ctx, relPath, rc)
	_ = rc.Close()
	if writeErr != nil {
		return "", domain.Entry{}, noopCleanup, writeErr
	}
	cleanup := func(ctx context.Context) error { return writable.Remove(ctx, relPath) }
	if info, err := src.Stat(ctx, canonical); err == nil && info.Size != written {
		_ = cleanup(ctx)
		return "", domain.Entry{}, noopCleanup, fmt.Errorf("copy %q: short write %d/%d bytes", name, written, info.Size)
	}
	entry, err := index.StatEntry(ctx, dstProv, relPath)
	if err != nil {
		_ = cleanup(ctx)
		return "", domain.Entry{}, noopCleanup, err
	}
	applyItemTime(&entry, item)
	return local.ProviderName, entry, cleanup, nil
}

// uniqueDest returns relPath, or the first "name (n).ext" variant that does not
// already exist under the provider, so a copy/move never overwrites a file.
func (s *Service) uniqueDest(ctx context.Context, prov kernel.StorageProvider, relPath string) string {
	if _, err := prov.Stat(ctx, relPath); err != nil {
		return relPath
	}
	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := prov.Stat(ctx, cand); err != nil {
			return cand
		}
	}
}

// applyItemTime overrides the entry's mod time with the source's creation time
// so asset.created_at reflects the original (Eagle btime / Billfish create_time)
// rather than the copy time. Zero time leaves the filesystem mod time.
func applyItemTime(e *domain.Entry, item ImportItem) {
	if !item.CreatedAt.IsZero() {
		e.ModTime = item.CreatedAt
	}
}

// hashFile returns the hex SHA-256 of the file at path read through p.
func hashFile(ctx context.Context, p kernel.StorageProvider, path string) (string, error) {
	rc, err := p.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", fmt.Errorf("hash %q: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// removePath deletes an absolute source path (move mode, after a successful
// import). It is separate so the service never calls os directly.
func removePath(path string) error { return os.Remove(path) }
