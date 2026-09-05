package kernel

import (
	"context"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// Store is the persistence contract. It is implemented by internal/store today
// and can be reimplemented on Postgres later without touching callers.
type Store interface {
	EnsureOwner(ctx context.Context, owner domain.OwnerID) error
	UpsertAsset(ctx context.Context, a domain.Asset) error
	ListAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error)
	GetAsset(ctx context.Context, owner domain.OwnerID, id domain.AssetID) (domain.Asset, error)

	// SearchAssets performs FTS5 full-text search. ftsQuery is a pre-built
	// FTS5 MATCH expression (produced by the search package parser).
	SearchAssets(ctx context.Context, owner domain.OwnerID, ftsQuery string, limit, offset int) ([]domain.Asset, error)

	// IndexPalette stores an asset's extracted palette: the palette and dominant
	// color as extracted-layer annotations, plus the searchable color index. The
	// asset is addressed by its natural key so the canonical row id is resolved
	// even when a re-scan generated a fresh id that was discarded on upsert.
	IndexPalette(ctx context.Context, owner domain.OwnerID, provider, path string, pal color.Palette) error
	// SearchByColor returns assets whose palette contains a swatch within tol
	// (CIEDE2000) of target, nearest first, capped at limit.
	SearchByColor(ctx context.Context, owner domain.OwnerID, target color.Lab, tol float64, limit int) ([]domain.ColorMatch, error)

	// IndexMetadata stores extracted file-embedded metadata (EXIF/IPTC/XMP)
	// as extracted-layer annotations, and updates asset.created_at when the
	// metadata contains an embedded capture time that predates the filesystem
	// timestamp. The asset is addressed by its natural key.
	IndexMetadata(ctx context.Context, owner domain.OwnerID, provider, path string, md domain.ExtractedMetadata) error

	// Batch metadata updates over a set of assets.
	BatchUpdateRating(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID, rating int) error
	BatchUpdateColor(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID, color string) error
	BatchUpdateDisplayName(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID, displayName string) error
	BatchRenameDisplayNames(ctx context.Context, owner domain.OwnerID, renames map[domain.AssetID]string) error
	BatchMoveToFolder(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID, folderID string) error

	// Trash lifecycle: soft-delete, restore, list, and permanent purge.
	BatchTrashAssets(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) error
	BatchRestoreAssets(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) error
	ListTrashedAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error)
	PurgeTrash(ctx context.Context, owner domain.OwnerID, retentionDays int) error

	// Tags: CRUD plus batch (un)tagging of assets.
	CreateTag(ctx context.Context, t domain.Tag) error
	ListTags(ctx context.Context, owner domain.OwnerID) ([]domain.Tag, error)
	DeleteTag(ctx context.Context, owner domain.OwnerID, id domain.TagID) error
	BatchAddTags(ctx context.Context, owner domain.OwnerID, assetIDs []domain.AssetID, tagIDs []domain.TagID) error
	BatchRemoveTags(ctx context.Context, owner domain.OwnerID, assetIDs []domain.AssetID, tagID domain.TagID) error
	ListAssetTags(ctx context.Context, assetID domain.AssetID) ([]domain.Tag, error)

	// Folders: virtual organization tree.
	CreateFolder(ctx context.Context, f domain.Folder) error
	ListFolders(ctx context.Context, owner domain.OwnerID) ([]domain.Folder, error)
	DeleteFolder(ctx context.Context, owner domain.OwnerID, id domain.FolderID) error

	// Duplicates: exact (SHA-256) and perceptual (pHash) duplicate detection.
	FindExactDuplicates(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.DuplicateGroup, error)
	IndexPHash(ctx context.Context, owner domain.OwnerID, provider, path string, phash uint64) error
	FindSimilarByPHash(ctx context.Context, owner domain.OwnerID, threshold int) ([]domain.SimilarGroup, error)

	Close() error
}
