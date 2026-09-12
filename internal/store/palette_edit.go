package store

import (
	"context"
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// AddAssetColor appends one manually-chosen swatch to an asset's palette and
// marks the palette user-curated (assets.palette_manual = 1), so a later scan or
// thumbnail re-extract leaves it intact (issue #62). The new swatch takes the
// next ord after the current maximum (ord 0 when the palette was empty); its
// weight is 0 because a hand-picked color carries no pixel fraction. Returns the
// full updated palette, dominant-first, in one transaction.
func (s *SQLite) AddAssetColor(ctx context.Context, owner domain.OwnerID, id domain.AssetID, rgb color.RGB) ([]color.Swatch, error) {
	return s.editPaletteTx(ctx, owner, id, func(q *db.Queries, aid string) error {
		rows, err := q.ListAssetColorsFull(ctx, db.ListAssetColorsFullParams{AssetID: aid, OwnerID: owner.String()})
		if err != nil {
			return fmt.Errorf("list colors: %w", err)
		}
		next := 0
		for _, r := range rows {
			if int(r.Ord) >= next {
				next = int(r.Ord) + 1
			}
		}
		lab := rgb.Lab()
		if err := q.InsertAssetColor(ctx, db.InsertAssetColorParams{
			AssetID: aid, OwnerID: owner.String(), Ord: int64(next),
			Hex: rgb.Hex(), L: lab.L, A: lab.A, B: lab.B, Weight: 0,
		}); err != nil {
			return fmt.Errorf("insert color: %w", err)
		}
		return nil
	})
}

// UpdateAssetColor repoints the swatch at ord to rgb (recomputing its Lab
// coordinates) and marks the palette user-curated (issue #62). It returns
// domain.ErrNotFound when the asset has no swatch at ord. Returns the full
// updated palette in one transaction.
func (s *SQLite) UpdateAssetColor(ctx context.Context, owner domain.OwnerID, id domain.AssetID, ord int, rgb color.RGB) ([]color.Swatch, error) {
	return s.editPaletteTx(ctx, owner, id, func(q *db.Queries, aid string) error {
		lab := rgb.Lab()
		n, err := q.UpdateAssetColorAt(ctx, db.UpdateAssetColorAtParams{
			Hex: rgb.Hex(), L: lab.L, A: lab.A, B: lab.B,
			AssetID: aid, OwnerID: owner.String(), Ord: int64(ord),
		})
		if err != nil {
			return fmt.Errorf("update color: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("swatch ord %d: %w", ord, domain.ErrNotFound)
		}
		return nil
	})
}

// DeleteAssetColor removes the swatch at ord and renumbers the survivors to a
// contiguous 0..N-1, preserving their relative order, so ord 0 always names the
// dominant color and no gap is left (deleting ord 0 promotes the old ord 1). It
// marks the palette user-curated (issue #62) and returns domain.ErrNotFound when
// no swatch has that ord. Returns the full updated palette in one transaction.
func (s *SQLite) DeleteAssetColor(ctx context.Context, owner domain.OwnerID, id domain.AssetID, ord int) ([]color.Swatch, error) {
	return s.editPaletteTx(ctx, owner, id, func(q *db.Queries, aid string) error {
		rows, err := q.ListAssetColorsFull(ctx, db.ListAssetColorsFullParams{AssetID: aid, OwnerID: owner.String()})
		if err != nil {
			return fmt.Errorf("list colors: %w", err)
		}
		found := false
		for _, r := range rows {
			if int(r.Ord) == ord {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("swatch ord %d: %w", ord, domain.ErrNotFound)
		}
		// Rebuild the survivors with contiguous ords. Delete-all-then-reinsert
		// (rather than an ord-shifting UPDATE) sidesteps a transient primary-key
		// collision on (asset_id, ord) and mirrors writePaletteTx's rebuild.
		if err := q.DeleteAssetColors(ctx, aid); err != nil {
			return fmt.Errorf("clear colors: %w", err)
		}
		next := 0
		for _, r := range rows {
			if int(r.Ord) == ord {
				continue
			}
			if err := q.InsertAssetColor(ctx, db.InsertAssetColorParams{
				AssetID: aid, OwnerID: owner.String(), Ord: int64(next),
				Hex: r.Hex, L: r.L, A: r.A, B: r.B, Weight: r.Weight,
			}); err != nil {
				return fmt.Errorf("renumber color: %w", err)
			}
			next++
		}
		return nil
	})
}

// editPaletteTx runs mutate against the asset's asset_colors rows, flips
// palette_manual to 1, and returns the resulting palette — all in one
// transaction so a manual edit and its curated flag can never diverge.
func (s *SQLite) editPaletteTx(ctx context.Context, owner domain.OwnerID, id domain.AssetID, mutate func(q *db.Queries, aid string) error) ([]color.Swatch, error) {
	aid := id.String()
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin palette edit tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	if err := mutate(q, aid); err != nil {
		return nil, err
	}
	if err := q.SetPaletteManual(ctx, db.SetPaletteManualParams{PaletteManual: 1, ID: aid, OwnerID: owner.String()}); err != nil {
		return nil, fmt.Errorf("mark palette manual: %w", err)
	}
	swatches, err := listSwatchesTx(ctx, q, owner, aid)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit palette edit: %w", err)
	}
	return swatches, nil
}

// listSwatchesTx reads an asset's swatches (ord-ascending) inside q's
// transaction, mirroring ListAssetColors so every edit returns the same shape
// the GET /colors endpoint does.
func listSwatchesTx(ctx context.Context, q *db.Queries, owner domain.OwnerID, aid string) ([]color.Swatch, error) {
	rows, err := q.GetAssetColors(ctx, db.GetAssetColorsParams{AssetID: aid, OwnerID: owner.String()})
	if err != nil {
		return nil, fmt.Errorf("list asset colors %s: %w", aid, err)
	}
	out := make([]color.Swatch, 0, len(rows))
	for _, r := range rows {
		rgb, err := color.ParseHex(r.Hex)
		if err != nil {
			continue // stored hex is always canonical; skip if somehow malformed
		}
		out = append(out, color.Swatch{RGB: rgb, Weight: r.Weight})
	}
	return out, nil
}
