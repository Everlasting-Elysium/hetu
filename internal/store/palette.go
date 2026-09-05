package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// IndexPalette persists pal for the asset identified by (owner, provider, path):
// the full palette and dominant color as extracted-layer annotations, plus one
// asset_colors row per swatch for search. All writes share a transaction so a
// re-scan never leaves a half-updated index. The natural key resolves the
// canonical row id even when a re-scan generated a fresh id discarded on upsert.
func (s *SQLite) IndexPalette(ctx context.Context, owner domain.OwnerID, provider, path string, pal color.Palette) error {
	if len(pal) == 0 {
		return nil
	}
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin palette tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	id, err := qtx.AssetIDByPath(ctx, db.AssetIDByPathParams{
		OwnerID: owner.String(), Provider: provider, StoragePath: path,
	})
	if err != nil {
		return fmt.Errorf("resolve asset %q: %w", path, err)
	}
	if err := upsertPaletteAnnotations(ctx, qtx, id, pal); err != nil {
		return err
	}
	if err := qtx.DeleteAssetColors(ctx, id); err != nil {
		return fmt.Errorf("clear colors: %w", err)
	}
	for ord, sw := range pal {
		lab := sw.Lab()
		if err := qtx.InsertAssetColor(ctx, db.InsertAssetColorParams{
			AssetID: id, OwnerID: owner.String(), Ord: int64(ord), Hex: sw.Hex(),
			L: lab.L, A: lab.A, B: lab.B, Weight: sw.Weight,
		}); err != nil {
			return fmt.Errorf("insert color: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit palette: %w", err)
	}
	return nil
}

func upsertPaletteAnnotations(ctx context.Context, q *db.Queries, id string, pal color.Palette) error {
	paletteJSON, err := json.Marshal(pal)
	if err != nil {
		return fmt.Errorf("marshal palette: %w", err)
	}
	dominantJSON, err := json.Marshal(pal[0].Hex())
	if err != nil {
		return fmt.Errorf("marshal dominant: %w", err)
	}
	now := time.Now().Unix()
	anns := []db.UpsertAnnotationParams{
		{AssetID: id, Layer: string(domain.LayerExtracted), Key: domain.KeyPalette, Value: string(paletteJSON), CreatedAt: now},
		{AssetID: id, Layer: string(domain.LayerExtracted), Key: domain.KeyDominant, Value: string(dominantJSON), CreatedAt: now},
	}
	for _, a := range anns {
		if err := q.UpsertAnnotation(ctx, a); err != nil {
			return fmt.Errorf("upsert annotation %q: %w", a.Key, err)
		}
	}
	return nil
}

// SearchByColor loads the owner's color index, computes the CIEDE2000 distance
// from target to each asset's nearest swatch, and returns assets within tol,
// nearest first. Distance cannot run in SQL, so ranking happens in Go over the
// precomputed Lab index; full asset rows are fetched only for the matches.
func (s *SQLite) SearchByColor(ctx context.Context, owner domain.OwnerID, target color.Lab, tol float64, limit int) ([]domain.ColorMatch, error) {
	cands, err := s.q.ColorCandidates(ctx, owner.String())
	if err != nil {
		return nil, fmt.Errorf("color candidates: %w", err)
	}
	type hit struct {
		id, hex string
		dist    float64
	}
	best := make(map[string]hit)
	for _, c := range cands {
		d := color.Distance(target, color.Lab{L: c.L, A: c.A, B: c.B})
		if d > tol {
			continue
		}
		if h, ok := best[c.AssetID]; !ok || d < h.dist {
			best[c.AssetID] = hit{id: c.AssetID, hex: c.Hex, dist: d}
		}
	}
	hits := make([]hit, 0, len(best))
	for _, h := range best {
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist
		}
		return hits[i].id < hits[j].id
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	if len(hits) == 0 {
		return []domain.ColorMatch{}, nil
	}

	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.id
	}
	rows, err := s.q.AssetsByIDs(ctx, db.AssetsByIDsParams{OwnerID: owner.String(), Ids: ids})
	if err != nil {
		return nil, fmt.Errorf("load matched assets: %w", err)
	}
	byID := make(map[string]domain.Asset, len(rows))
	for _, r := range rows {
		a, err := rowToAsset(r)
		if err != nil {
			return nil, err
		}
		byID[a.ID.String()] = a
	}
	matches := make([]domain.ColorMatch, 0, len(hits))
	for _, h := range hits {
		if a, ok := byID[h.id]; ok {
			matches = append(matches, domain.ColorMatch{Asset: a, Hex: h.hex, Distance: h.dist})
		}
	}
	return matches, nil
}
