// Package index walks a storage provider and indexes supported assets in place
// (files are never copied). For each file it resolves an asset handler, extracts
// metadata, hashes the content, renders a thumbnail, and upserts the record.
package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// Indexer indexes one owner's assets from a storage provider.
type Indexer struct {
	k     *kernel.Kernel
	owner domain.OwnerID
}

// New returns an indexer bound to a kernel and owner.
func New(k *kernel.Kernel, owner domain.OwnerID) *Indexer {
	return &Indexer{k: k, owner: owner}
}

// ScanResult summarises a scan.
type ScanResult struct {
	Indexed     int `json:"indexed"`
	Skipped     int `json:"skipped"`
	Missing     int `json:"missing"`
	Reconnected int `json:"reconnected"`
}

// Scan walks providerName from root, indexing every supported file.
func (ix *Indexer) Scan(ctx context.Context, providerName, root string) (ScanResult, error) {
	provider, ok := ix.k.Storage.Get(providerName)
	if !ok {
		return ScanResult{}, fmt.Errorf("scan: provider %q: %w", providerName, domain.ErrNotFound)
	}
	if err := ix.k.Store.EnsureOwner(ctx, ix.owner); err != nil {
		return ScanResult{}, err
	}
	var res ScanResult
	// Detect missing files before walking: a file moved to a new path then
	// reconnects (by content hash) to its now-missing record in this same scan
	// instead of being indexed as a duplicate (see detectMissing, indexOne).
	ix.detectMissing(ctx, provider, &res)
	if err := ix.walk(ctx, provider, root, &res); err != nil {
		return res, err
	}
	ix.k.Events.Publish(ctx, kernel.Event{Type: kernel.EventScanFinished, Data: res})
	return res, nil
}

// detectMissing marks every live asset for provider p whose backing file is no
// longer present in storage. It runs before the walk so a moved file can
// reconnect by hash to its now-missing record in the same scan (see indexOne);
// a file that reappears at its old path is likewise cleared by that reconnect.
func (ix *Indexer) detectMissing(ctx context.Context, p kernel.StorageProvider, res *ScanResult) {
	assets, err := ix.k.Store.ListLiveAssetsByProvider(ctx, ix.owner, p.Name())
	if err != nil {
		ix.k.Log.WarnContext(ctx, "detect missing: list assets", slog.Any("err", err))
		return
	}
	var missingIDs []domain.AssetID
	for _, a := range assets {
		if _, err := p.Stat(ctx, a.StoragePath); err != nil {
			missingIDs = append(missingIDs, a.ID)
		}
	}
	if len(missingIDs) == 0 {
		return
	}
	if err := ix.k.Store.MarkAssetsMissing(ctx, ix.owner, missingIDs); err != nil {
		ix.k.Log.WarnContext(ctx, "detect missing: mark", slog.Any("err", err))
		return
	}
	res.Missing = len(missingIDs)
	ix.k.Log.InfoContext(ctx, "marked assets missing", slog.Int("count", len(missingIDs)))
}

func (ix *Indexer) walk(ctx context.Context, p kernel.StorageProvider, path string, res *ScanResult) error {
	entries, err := p.List(ctx, path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.IsDir {
			// Never descend into the hetu-managed tree: version copies live
			// under it (issue #58) and must not be indexed as new assets. This
			// exclusion has no safety net downstream, so it is load-bearing.
			if e.Name == domain.ManagedDirName {
				continue
			}
			if err := ix.walk(ctx, p, e.Path, res); err != nil {
				return err
			}
			continue
		}
		reconnected, err := ix.indexOne(ctx, p, e)
		if err != nil {
			ix.k.Log.WarnContext(ctx, "index skip",
				slog.String("path", e.Path), slog.Any("err", err))
			res.Skipped++
			continue
		}
		if reconnected {
			res.Reconnected++
			continue
		}
		res.Indexed++
	}
	return nil
}

// indexOne indexes a single file. It returns reconnected=true when the file's
// content hash matched a missing asset and that asset was repointed to this
// path (no new record created); reconnected=false for a normal index/upsert.
func (ix *Indexer) indexOne(ctx context.Context, p kernel.StorageProvider, e domain.Entry) (bool, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(e.Name), "."))
	handler, ok := ix.k.Assets.HandlerFor(ext)
	if !ok {
		return false, domain.ErrUnsupported
	}

	meta, err := ix.extract(ctx, p, e.Path, handler)
	if err != nil {
		return false, err
	}
	hash, err := ix.hash(ctx, p, e.Path)
	if err != nil {
		return false, err
	}
	// Hash-based auto-reconnect: if a missing asset has this content hash, reuse
	// its identity (repoint its path, clear missing_at) instead of creating a
	// new record — this is a moved or remounted file, not a new one.
	candidates, err := ix.k.Store.ListMissingAssetsByHash(ctx, ix.owner, hash)
	if err != nil {
		ix.k.Log.WarnContext(ctx, "auto-reconnect lookup", slog.Any("err", err))
	}
	if len(candidates) > 0 {
		old := candidates[0]
		if err := ix.k.Store.RelocateAsset(ctx, ix.owner, old.ID, p.Name(), e.Path); err != nil {
			return false, fmt.Errorf("auto-reconnect relocate: %w", err)
		}
		ix.k.Log.InfoContext(ctx, "auto-reconnected",
			slog.String("asset", old.ID.String()),
			slog.String("old_path", old.StoragePath),
			slog.String("new_path", e.Path))
		return true, nil
	}
	id := uuid.Must(uuid.NewV7()).String()
	thumbPath, err := ix.thumbnail(ctx, p, e.Path, handler, id)
	if err != nil {
		return false, err
	}
	assetID, err := domain.NewAssetID(id)
	if err != nil {
		return false, err
	}
	asset := domain.Asset{
		ID:          assetID,
		Owner:       ix.owner,
		Kind:        handler.Kind(),
		Provider:    p.Name(),
		StoragePath: e.Path,
		Name:        e.Name,
		Ext:         ext,
		Size:        e.Size,
		Hash:        hash,
		ThumbPath:   thumbPath,
		Width:       meta.Width,
		Height:      meta.Height,
		CreatedAt:   e.ModTime,
		IndexedAt:   time.Now().UTC(),
	}
	if err := ix.k.Store.UpsertAsset(ctx, asset); err != nil {
		return false, err
	}
	// Palette and pHash extraction run after upsert so the asset row exists;
	// they enhance the record and never fail the index.
	ix.indexPalette(ctx, p, e.Path, handler)
	ix.indexPHash(ctx, p, e.Path, handler)
	// Metadata extraction (EXIF/IPTC/XMP) runs after upsert for the same
	// reason; embedded capture time may update asset.created_at.
	ix.indexMetadata(ctx, p, e.Path, handler)
	// Announce the indexed asset so subscribers (e.g. AI tagging) can react.
	// The bus is synchronous; handlers must not block (see internal/ai).
	ix.k.Events.Publish(ctx, kernel.Event{Type: kernel.EventAssetIndexed, Data: asset})
	return false, nil
}

func (ix *Indexer) extract(ctx context.Context, p kernel.StorageProvider, path string, h kernel.AssetHandler) (domain.Meta, error) {
	rc, err := p.Open(ctx, path)
	if err != nil {
		return domain.Meta{}, err
	}
	defer rc.Close()
	return h.Extract(ctx, rc)
}

func (ix *Indexer) hash(ctx context.Context, p kernel.StorageProvider, path string) (string, error) {
	rc, err := p.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, rc); err != nil {
		return "", fmt.Errorf("hash %q: %w", path, err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func (ix *Indexer) thumbnail(ctx context.Context, p kernel.StorageProvider, path string, h kernel.AssetHandler, id string) (string, error) {
	rc, err := p.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	if err := os.MkdirAll(ix.k.ThumbDir, 0o755); err != nil {
		return "", fmt.Errorf("create thumb dir: %w", err)
	}
	thumbPath := filepath.Join(ix.k.ThumbDir, id+".jpg")
	out, err := os.Create(thumbPath)
	if err != nil {
		return "", fmt.Errorf("create thumb: %w", err)
	}
	genErr := h.Thumbnail(ctx, rc, out)
	closeErr := out.Close()
	if genErr != nil {
		_ = os.Remove(thumbPath)
		if errors.Is(genErr, domain.ErrNoThumbnail) {
			return "", nil
		}
		return "", fmt.Errorf("thumbnail %q: %w", path, genErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close thumb: %w", closeErr)
	}
	return thumbPath, nil
}
