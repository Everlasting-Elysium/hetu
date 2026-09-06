package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math/bits"
	"sort"
	"strconv"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// FindExactDuplicates returns groups of live assets sharing the same SHA-256
// content hash. Each group contains 2+ assets; the groups are ordered by
// descending member count. limit/offset paginate the group list, not the assets.
func (s *SQLite) FindExactDuplicates(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.DuplicateGroup, error) {
	hashes, err := s.q.ListDuplicateHashes(ctx, db.ListDuplicateHashesParams{
		OwnerID: owner.String(), Limit: int64(limit), Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list duplicate hashes: %w", err)
	}
	if len(hashes) == 0 {
		return []domain.DuplicateGroup{}, nil
	}
	groups := make([]domain.DuplicateGroup, 0, len(hashes))
	for _, h := range hashes {
		rows, err := s.q.ListAssetsByHash(ctx, db.ListAssetsByHashParams{
			OwnerID: owner.String(), Hash: h.Hash,
		})
		if err != nil {
			return nil, fmt.Errorf("list assets for hash %s: %w", h.Hash[:8], err)
		}
		assets, err := rowsToAssets(rows)
		if err != nil {
			return nil, err
		}
		groups = append(groups, domain.DuplicateGroup{Hash: h.Hash, Assets: assets})
	}
	return groups, nil
}

// ListAssetsByHash returns the owner's live assets whose content hash equals
// hash (oldest first). The import service uses it to skip content duplicates
// (same bytes reached via a different path) when the conflict policy is skip.
func (s *SQLite) ListAssetsByHash(ctx context.Context, owner domain.OwnerID, hash string) ([]domain.Asset, error) {
	rows, err := s.q.ListAssetsByHash(ctx, db.ListAssetsByHashParams{
		OwnerID: owner.String(), Hash: hash,
	})
	if err != nil {
		return nil, fmt.Errorf("list assets by hash: %w", err)
	}
	return rowsToAssets(rows)
}

// IndexPHash stores the perceptual hash for an asset as an extracted-layer
// annotation. The asset is addressed by its natural key (owner, provider, path)
// so the canonical row id resolves even when a re-scan generated a fresh id
// that was discarded on upsert.
func (s *SQLite) IndexPHash(ctx context.Context, owner domain.OwnerID, provider, path string, phash uint64) error {
	id, err := s.q.AssetIDByPath(ctx, db.AssetIDByPathParams{
		OwnerID: owner.String(), Provider: provider, StoragePath: path,
	})
	if err != nil {
		return fmt.Errorf("resolve asset %q for phash: %w", path, err)
	}
	valueJSON, err := json.Marshal(strconv.FormatUint(phash, 10))
	if err != nil {
		return fmt.Errorf("marshal phash: %w", err)
	}
	return s.q.UpsertAnnotation(ctx, db.UpsertAnnotationParams{
		AssetID:   id,
		Layer:     string(domain.LayerExtracted),
		Key:       domain.KeyPHash,
		Value:     string(valueJSON),
		CreatedAt: time.Now().Unix(),
	})
}

// FindSimilarByPHash loads all pHash annotations for the owner's live assets,
// computes pairwise hamming distances, and groups assets within the threshold.
// The hamming distance cannot run in SQL (it requires XOR + popcount on uint64),
// so ranking happens in Go over the precomputed annotations.
func (s *SQLite) FindSimilarByPHash(ctx context.Context, owner domain.OwnerID, threshold int) ([]domain.SimilarGroup, error) {
	rows, err := s.q.ListPHashAnnotations(ctx, owner.String())
	if err != nil {
		return nil, fmt.Errorf("list phash annotations: %w", err)
	}
	if len(rows) == 0 {
		return []domain.SimilarGroup{}, nil
	}
	type entry struct {
		id    string
		phash uint64
	}
	entries := make([]entry, 0, len(rows))
	for _, r := range rows {
		var s string
		if err := json.Unmarshal([]byte(r.Value), &s); err != nil {
			continue // skip malformed annotations
		}
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			continue
		}
		entries = append(entries, entry{id: r.AssetID, phash: v})
	}
	// pendingHit tracks a member candidate before its full Asset is resolved.
	type pendingHit struct {
		id   string
		dist int
	}
	// Build similarity groups: each asset is the anchor at most once. Once an
	// asset appears as a member it is excluded from becoming a new anchor, so
	// every asset appears in at most one group.
	used := make(map[string]bool, len(entries))
	var groups []domain.SimilarGroup
	for i, anchor := range entries {
		if used[anchor.id] {
			continue
		}
		var pending []pendingHit
		for j := i + 1; j < len(entries); j++ {
			other := entries[j]
			if used[other.id] {
				continue
			}
			dist := hammingDistance(anchor.phash, other.phash)
			if dist <= threshold {
				pending = append(pending, pendingHit{id: other.id, dist: dist})
				used[other.id] = true
			}
		}
		if len(pending) == 0 {
			continue
		}
		used[anchor.id] = true
		// Collect all ids for bulk fetch.
		ids := make([]string, 0, 1+len(pending))
		ids = append(ids, anchor.id)
		for _, p := range pending {
			ids = append(ids, p.id)
		}
		assetRows, err := s.q.AssetsByIDs(ctx, db.AssetsByIDsParams{
			OwnerID: owner.String(), Ids: ids,
		})
		if err != nil {
			return nil, fmt.Errorf("load similar assets: %w", err)
		}
		byID := make(map[string]domain.Asset, len(assetRows))
		for _, r := range assetRows {
			a, err := rowToAsset(r)
			if err != nil {
				return nil, err
			}
			byID[a.ID.String()] = a
		}
		anchorAsset, ok := byID[anchor.id]
		if !ok {
			continue // asset deleted between queries
		}
		resolved := make([]domain.SimilarHit, 0, len(pending))
		for _, p := range pending {
			if a, ok := byID[p.id]; ok {
				resolved = append(resolved, domain.SimilarHit{Asset: a, Distance: p.dist})
			}
		}
		if len(resolved) == 0 {
			continue
		}
		sort.Slice(resolved, func(i, j int) bool {
			return resolved[i].Distance < resolved[j].Distance
		})
		groups = append(groups, domain.SimilarGroup{Anchor: anchorAsset, Members: resolved})
	}
	return groups, nil
}

// hammingDistance returns the number of differing bits between two uint64 values.
func hammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}
