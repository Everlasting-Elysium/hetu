package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func TestSQLite_IndexMetadata(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	seedAsset(t, ctx, st, owner, "photo-1", "photos/sunset.jpg")

	exifTime := time.Date(2024, 6, 15, 18, 30, 0, 0, time.UTC)
	md := domain.ExtractedMetadata{
		Annotations: map[string]any{
			domain.KeyExifCameraMake:  "Canon",
			domain.KeyExifCameraModel: "EOS R5",
			domain.KeyExifISO:         800,
			domain.KeyExifFNumber:     "f/2.8",
			domain.KeyExifExposure:    "1/125",
			domain.KeyExifFocalLength: "50mm",
			domain.KeyExifGPSLatitude: 39.9042,
			domain.KeyExifGPSLongitude: 116.4074,
			domain.KeyExifDateTime:    exifTime.Format(time.RFC3339),
			domain.KeyIPTCKeywords:    []string{"sunset", "nature"},
			domain.KeyXMPCreator:      "John Doe",
		},
		DateTime: exifTime,
		Keywords: []string{"sunset", "nature"},
	}

	if err := st.IndexMetadata(ctx, owner, "local", "photos/sunset.jpg", md); err != nil {
		t.Fatalf("IndexMetadata: %v", err)
	}

	assertAnnotation(t, ctx, dbPath, "photo-1", domain.KeyExifCameraMake, `"Canon"`)
	assertAnnotation(t, ctx, dbPath, "photo-1", domain.KeyExifCameraModel, `"EOS R5"`)
	assertAnnotation(t, ctx, dbPath, "photo-1", domain.KeyExifISO, `800`)
	assertAnnotation(t, ctx, dbPath, "photo-1", domain.KeyExifFNumber, `"f/2.8"`)
	assertAnnotation(t, ctx, dbPath, "photo-1", domain.KeyExifGPSLatitude, `39.9042`)

	// IPTC keywords stored as JSON array.
	assertAnnotation(t, ctx, dbPath, "photo-1", domain.KeyIPTCKeywords, `["sunset","nature"]`)

	// XMP creator.
	assertAnnotation(t, ctx, dbPath, "photo-1", domain.KeyXMPCreator, `"John Doe"`)

	// EXIF datetime should have updated asset.created_at.
	assertCreatedAt(t, ctx, dbPath, "photo-1", exifTime)
}

func TestSQLite_IndexMetadata_empty_skips(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedAsset(t, ctx, st, owner, "empty-1", "empty.jpg")

	md := domain.ExtractedMetadata{Annotations: map[string]any{}}
	if err := st.IndexMetadata(ctx, owner, "local", "empty.jpg", md); err != nil {
		t.Fatalf("IndexMetadata with empty annotations: %v", err)
	}
}

func TestSQLite_IndexMetadata_re_index_updates(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	seedAsset(t, ctx, st, owner, "reindex-1", "reindex.jpg")

	md1 := domain.ExtractedMetadata{
		Annotations: map[string]any{domain.KeyExifISO: 400},
	}
	if err := st.IndexMetadata(ctx, owner, "local", "reindex.jpg", md1); err != nil {
		t.Fatalf("first index: %v", err)
	}
	assertAnnotation(t, ctx, dbPath, "reindex-1", domain.KeyExifISO, `400`)

	// Re-index with updated ISO — ON CONFLICT should update.
	md2 := domain.ExtractedMetadata{
		Annotations: map[string]any{domain.KeyExifISO: 1600},
	}
	if err := st.IndexMetadata(ctx, owner, "local", "reindex.jpg", md2); err != nil {
		t.Fatalf("re-index: %v", err)
	}
	assertAnnotation(t, ctx, dbPath, "reindex-1", domain.KeyExifISO, `1600`)
}

func assertAnnotation(t *testing.T, ctx context.Context, dbPath, assetID, key, wantValue string) {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	var val string
	err = raw.QueryRowContext(ctx,
		`SELECT value FROM annotations WHERE asset_id=? AND layer='extracted' AND "key"=?`,
		assetID, key).Scan(&val)
	if err != nil {
		t.Fatalf("read annotation %q for %s: %v", key, assetID, err)
	}

	// Normalize JSON for comparison (handle whitespace differences).
	var wantNorm, gotNorm json.RawMessage
	if err := json.Unmarshal([]byte(wantValue), &wantNorm); err != nil {
		t.Fatalf("unmarshal want %q: %v", wantValue, err)
	}
	if err := json.Unmarshal([]byte(val), &gotNorm); err != nil {
		t.Fatalf("unmarshal got %q: %v", val, err)
	}
	wantBytes, _ := json.Marshal(wantNorm)
	gotBytes, _ := json.Marshal(gotNorm)
	if string(wantBytes) != string(gotBytes) {
		t.Fatalf("annotation %q = %s, want %s", key, val, wantValue)
	}
}

func assertCreatedAt(t *testing.T, ctx context.Context, dbPath, assetID string, want time.Time) {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	var createdAt int64
	err = raw.QueryRowContext(ctx,
		`SELECT created_at FROM assets WHERE id=?`, assetID).Scan(&createdAt)
	if err != nil {
		t.Fatalf("read created_at for %s: %v", assetID, err)
	}
	got := time.Unix(createdAt, 0).UTC()
	if !got.Equal(want) {
		t.Fatalf("created_at = %v, want %v", got, want)
	}
}
