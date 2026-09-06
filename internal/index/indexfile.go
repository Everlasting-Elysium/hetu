package index

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// IndexFile indexes the single file described by e under providerName and
// returns the canonical stored asset. Unlike Scan it does not walk a directory:
// the import API and migration importers use it to ingest one file at a time and
// then attach tags/folders/rating/annotations to the returned asset.
//
// The asset is re-resolved by natural key (owner, provider, storage_path)
// rather than returned from the in-memory record, because UpsertAsset's
// ON CONFLICT clause preserves the existing id on a re-index and discards the
// freshly minted one — so only the DB holds the trustworthy identity.
func (ix *Indexer) IndexFile(ctx context.Context, providerName string, e domain.Entry) (domain.Asset, error) {
	p, ok := ix.k.Storage.Get(providerName)
	if !ok {
		return domain.Asset{}, fmt.Errorf("index file: provider %q: %w", providerName, domain.ErrNotFound)
	}
	if err := ix.k.Store.EnsureOwner(ctx, ix.owner); err != nil {
		return domain.Asset{}, err
	}
	if _, err := ix.indexOne(ctx, p, e); err != nil {
		return domain.Asset{}, fmt.Errorf("index file %q: %w", e.Path, err)
	}
	return ix.k.Store.GetAssetByPath(ctx, ix.owner, providerName, e.Path)
}

// StatEntry builds a domain.Entry for a single path via the provider's Stat, so
// callers can feed one file to IndexFile without a directory walk. Callers may
// override the returned ModTime (e.g. an importer injecting the source's
// creation time) before passing it on, since ModTime flows into created_at.
func StatEntry(ctx context.Context, p kernel.StorageProvider, path string) (domain.Entry, error) {
	info, err := p.Stat(ctx, path)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("stat %q: %w", path, err)
	}
	return domain.Entry{
		Name:    filepath.Base(path),
		Path:    path,
		IsDir:   info.IsDir,
		Size:    info.Size,
		ModTime: info.ModTime,
	}, nil
}
