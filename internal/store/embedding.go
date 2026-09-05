package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
	"github.com/Everlasting-Elysium/hetu/internal/vecmath"
)

// IndexEmbedding upserts a CLIP embedding for the given asset. Called only via
// the narrow ai.EmbedPersister interface, not through kernel.Store.
func (s *SQLite) IndexEmbedding(ctx context.Context, assetID domain.AssetID, embedding []float32, model string) error {
	if err := s.q.UpsertEmbedding(ctx, db.UpsertEmbeddingParams{
		AssetID:   assetID.String(),
		Embedding: vecmath.Float32sToBytes(embedding),
		Model:     model,
		CreatedAt: time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("index embedding %s: %w", assetID, err)
	}
	return nil
}

// GetEmbedding returns the stored CLIP vector for an asset, or
// domain.ErrNotFound.
func (s *SQLite) GetEmbedding(ctx context.Context, assetID domain.AssetID) ([]float32, error) {
	row, err := s.q.GetEmbedding(ctx, assetID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get embedding %s: %w", assetID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get embedding %s: %w", assetID, err)
	}
	v, err := vecmath.BytesToFloat32s(row.Embedding)
	if err != nil {
		return nil, fmt.Errorf("decode embedding %s: %w", assetID, err)
	}
	return v, nil
}

// SearchByEmbedding performs brute-force dot-product similarity search over all
// embeddings belonging to the owner's live (non-trashed) assets. It returns the
// top-limit results sorted by descending similarity. excludeID, when non-zero,
// is omitted from results (used by visual-similar to exclude the query asset,
// which always scores 1.0 against itself).
func (s *SQLite) SearchByEmbedding(ctx context.Context, owner domain.OwnerID, query []float32, excludeID domain.AssetID, limit int) ([]domain.SimilarityMatch, error) {
	rows, err := s.q.ListOwnerEmbeddings(ctx, owner.String())
	if err != nil {
		return nil, fmt.Errorf("list embeddings: %w", err)
	}
	exclude := excludeID.String()

	type ranked struct {
		assetID string
		sim     float64
	}
	results := make([]ranked, 0, len(rows))
	var skipped int
	for _, r := range rows {
		if exclude != "" && r.AssetID == exclude {
			continue
		}
		v, err := vecmath.BytesToFloat32s(r.Embedding)
		if err != nil {
			skipped++
			continue
		}
		if len(v) != len(query) {
			skipped++
			continue // dimension mismatch (model upgrade)
		}
		results = append(results, ranked{assetID: r.AssetID, sim: vecmath.Dot(query, v)})
	}
	if skipped > 0 {
		slog.Debug("embedding search skipped rows",
			slog.Int("skipped", skipped), slog.Int("total", len(rows)))
	}

	sort.Slice(results, func(i, j int) bool { return results[i].sim > results[j].sim })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	matches := make([]domain.SimilarityMatch, 0, len(results))
	for _, m := range results {
		asset, err := s.loadLiveAssetByID(ctx, m.assetID)
		if err != nil {
			continue // asset deleted/trashed between listing and loading
		}
		matches = append(matches, domain.SimilarityMatch{Asset: asset, Similarity: m.sim})
	}
	return matches, nil
}

// loadLiveAssetByID loads a single live (non-trashed) asset by id.
func (s *SQLite) loadLiveAssetByID(ctx context.Context, id string) (domain.Asset, error) {
	const q = `SELECT id, owner_id, kind, provider, storage_path, name, ext,
	       size, hash, thumb_path, width, height, created_at, indexed_at,
	       deleted_at, rating, color, display_name, folder_id
	FROM assets WHERE id = ? AND deleted_at IS NULL`
	var r db.Asset
	err := s.sqldb.QueryRowContext(ctx, q, id).Scan(
		&r.ID, &r.OwnerID, &r.Kind, &r.Provider, &r.StoragePath,
		&r.Name, &r.Ext, &r.Size, &r.Hash, &r.ThumbPath,
		&r.Width, &r.Height, &r.CreatedAt, &r.IndexedAt,
		&r.DeletedAt, &r.Rating, &r.Color, &r.DisplayName, &r.FolderID,
	)
	if err != nil {
		return domain.Asset{}, err
	}
	return rowToAsset(r)
}
