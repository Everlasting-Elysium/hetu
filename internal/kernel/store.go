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
	// ListAssetsFiltered narrows ListAssets by folder, tag, and minimum rating
	// (see domain.AssetFilter); zero-value filter fields are ignored.
	ListAssetsFiltered(ctx context.Context, owner domain.OwnerID, f domain.AssetFilter, limit, offset int) ([]domain.Asset, error)
	GetAsset(ctx context.Context, owner domain.OwnerID, id domain.AssetID) (domain.Asset, error)
	// GetAssetByPath resolves an asset by its natural key (owner, provider,
	// storage_path). Callers that index a single file use it to obtain the
	// canonical row id, which UpsertAsset's ON CONFLICT clause preserves.
	GetAssetByPath(ctx context.Context, owner domain.OwnerID, provider, path string) (domain.Asset, error)

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

	// UpsertAnnotation writes a single layered annotation for an asset, keyed by
	// (asset_id, layer, key). Used by the import/migration path to persist a
	// source URL (extracted layer) or a migrated note (manual layer). Value is a
	// JSON-serialized payload, matching the extracted/ai writers.
	UpsertAnnotation(ctx context.Context, owner domain.OwnerID, a domain.Annotation) error

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

	// Missing-file detection and relocate (issue #45).
	ListMissingAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error)
	ListLiveAssetsByProvider(ctx context.Context, owner domain.OwnerID, provider string) ([]domain.Asset, error)
	ListMissingAssetsByHash(ctx context.Context, owner domain.OwnerID, hash string) ([]domain.Asset, error)
	MarkAssetsMissing(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) error
	MarkAssetsFound(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) error
	RelocateAsset(ctx context.Context, owner domain.OwnerID, id domain.AssetID, provider, newPath string) error
	RebaseAssets(ctx context.Context, owner domain.OwnerID, provider, oldPrefix, newPrefix string) error

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
	// ListAssetsByHash returns the owner's live assets with the given content
	// hash (oldest first); the import service uses it to skip content dupes.
	ListAssetsByHash(ctx context.Context, owner domain.OwnerID, hash string) ([]domain.Asset, error)
	IndexPHash(ctx context.Context, owner domain.OwnerID, provider, path string, phash uint64) error
	FindSimilarByPHash(ctx context.Context, owner domain.OwnerID, threshold int) ([]domain.SimilarGroup, error)

	// Shares: create a share link and resolve one by its public token. The
	// share-creation/serving API (issue #4) builds on these persistence methods.
	CreateShare(ctx context.Context, sh domain.Share) error
	GetShareByToken(ctx context.Context, token string) (domain.Share, error)

	// Jobs: persisted background-task queue. Execution is owned by the job
	// runtime (kernel.JobQueue and issues #8/#9); these methods only persist.
	EnqueueJob(ctx context.Context, j domain.Job) error
	UpdateJobStatus(ctx context.Context, owner domain.OwnerID, id domain.JobID, status domain.JobStatus) error
	// UpdateJob transitions a job's status and replaces its payload together, so
	// a long-running import can persist progress counts (JSON) as it advances.
	UpdateJob(ctx context.Context, owner domain.OwnerID, id domain.JobID, status domain.JobStatus, payload string) error
	ListJobs(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Job, error)

	// Embeddings: CLIP vector retrieval and brute-force similarity search.
	// Write path (IndexEmbedding) lives on *store.SQLite only, accessed via the
	// narrow ai.EmbedPersister interface — mirroring PersistAITagResult.
	// excludeID (zero value = none) omits an asset from results, so visual-
	// similar can drop the query asset that always scores 1.0 against itself.
	GetEmbedding(ctx context.Context, assetID domain.AssetID) ([]float32, error)
	SearchByEmbedding(ctx context.Context, owner domain.OwnerID, query []float32, excludeID domain.AssetID, limit int) ([]domain.SimilarityMatch, error)

	Close() error
}
