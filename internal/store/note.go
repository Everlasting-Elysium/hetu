package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// UpsertManualCaption writes or replaces the manual-layer caption for an asset.
// The text is JSON-encoded for consistency with AI/extracted caption values.
func (s *SQLite) UpsertManualCaption(ctx context.Context, owner domain.OwnerID, id domain.AssetID, text string) error {
	if _, err := s.GetAsset(ctx, owner, id); err != nil {
		return fmt.Errorf("upsert manual caption: %w", err)
	}
	valJSON, err := json.Marshal(text)
	if err != nil {
		return fmt.Errorf("marshal manual caption: %w", err)
	}
	if err := s.q.UpsertAnnotation(ctx, db.UpsertAnnotationParams{
		AssetID:   id.String(),
		Layer:     string(domain.LayerManual),
		Key:       domain.KeyCaption,
		Value:     string(valJSON),
		Model:     "",
		CreatedAt: time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("upsert manual caption %s: %w", id, err)
	}
	return nil
}

// DeleteManualCaption removes the manual-layer caption for an asset.
func (s *SQLite) DeleteManualCaption(ctx context.Context, owner domain.OwnerID, id domain.AssetID) error {
	if _, err := s.GetAsset(ctx, owner, id); err != nil {
		return fmt.Errorf("delete manual caption: %w", err)
	}
	if err := s.q.DeleteAnnotation(ctx, db.DeleteAnnotationParams{
		AssetID: id.String(),
		Layer:   string(domain.LayerManual),
		Key:     domain.KeyCaption,
	}); err != nil {
		return fmt.Errorf("delete manual caption %s: %w", id, err)
	}
	return nil
}

// ListManualCaptions returns the manual-layer caption for each of the owner's
// assets that has one. The returned map is keyed by asset id string; assets
// without a note are absent. Owner scoping is enforced by the query's join.
func (s *SQLite) ListManualCaptions(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) (map[string]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q.ListManualCaptions(ctx, db.ListManualCaptionsParams{
		OwnerID: owner.String(),
		Ids:     idStrings(ids),
	})
	if err != nil {
		return nil, fmt.Errorf("list manual captions: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.AssetID] = unquoteJSON(r.Value)
	}
	return out, nil
}

// unquoteJSON strips JSON string encoding from a value that was stored via
// json.Marshal(string). If the value is not a valid JSON string, it is returned
// as-is (defensive against legacy or manually-inserted rows).
func unquoteJSON(raw string) string {
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return raw
	}
	return s
}
