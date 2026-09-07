package dam_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// pngThumbBytes builds a body opening with the real PNG signature so
// http.DetectContentType classifies it as image/png (see ensureImagePNGorJPEG).
func pngThumbBytes(payload string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), []byte(payload)...)
}

// buildThumbUpload encodes content as a multipart body with a single "file"
// field, returning the body and its Content-Type header.
func buildThumbUpload(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// upsertThumbAsset indexes a minimal model asset so the thumb endpoint has a row
// to repoint, returning its id.
func upsertThumbAsset(t *testing.T, st kernel.Store, owner domain.OwnerID, id string) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(context.Background(), domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindModel, Provider: "local",
		StoragePath: id + ".stl", Name: id + ".stl", Ext: "stl",
		Size: 1, Hash: "h-" + id, CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert asset: %v", err)
	}
	return aid
}

func TestUploadThumb_Success(t *testing.T) {
	srv, owner, st := newTestServer(t)
	aid := upsertThumbAsset(t, st, owner, "thumb-ok")
	png := pngThumbBytes("rendered-3d-preview")

	body, ct := buildThumbUpload(t, "preview.png", png)
	resp, err := http.Post(srv.URL+"/api/dam/assets/thumb-ok/thumb", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, b)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["thumb"] == "" {
		t.Fatalf("response thumb path empty, want a path")
	}

	asset, err := st.GetAsset(context.Background(), owner, aid)
	if err != nil {
		t.Fatal(err)
	}
	if asset.ThumbPath != out["thumb"] {
		t.Errorf("stored thumb_path = %q, want %q", asset.ThumbPath, out["thumb"])
	}
	saved, err := os.ReadFile(asset.ThumbPath)
	if err != nil {
		t.Fatalf("read saved thumb: %v", err)
	}
	if !bytes.Equal(saved, png) {
		t.Errorf("saved thumb = %d bytes, want the uploaded %d PNG bytes", len(saved), len(png))
	}
}

func TestUploadThumb_MissingAsset404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body, ct := buildThumbUpload(t, "preview.png", pngThumbBytes("x"))
	resp, err := http.Post(srv.URL+"/api/dam/assets/ghost/thumb", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for missing asset", resp.StatusCode)
	}
}

func TestUploadThumb_RejectsNonImage(t *testing.T) {
	srv, owner, st := newTestServer(t)
	upsertThumbAsset(t, st, owner, "thumb-bad")
	body, ct := buildThumbUpload(t, "notes.txt", []byte("plain text, definitely not an image file"))
	resp, err := http.Post(srv.URL+"/api/dam/assets/thumb-bad/thumb", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for non-image body", resp.StatusCode)
	}
}

func TestUploadThumb_OversizedBody(t *testing.T) {
	srv, owner, st := newTestServer(t)
	aid := upsertThumbAsset(t, st, owner, "thumb-big")
	// 11 MiB (> maxThumbUpload) behind a valid PNG header: MaxBytesReader must
	// reject it before it is ever written or recorded. A mid-upload connection
	// close (transport error) is an equally valid rejection.
	big := pngThumbBytes(strings.Repeat("A", 11<<20))
	body, ct := buildThumbUpload(t, "huge.png", big)

	resp, err := http.Post(srv.URL+"/api/dam/assets/thumb-big/thumb", ct, body)
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 for oversized body", resp.StatusCode)
		}
	}
	asset, err := st.GetAsset(context.Background(), owner, aid)
	if err != nil {
		t.Fatal(err)
	}
	if asset.ThumbPath != "" {
		t.Errorf("thumb_path = %q, want empty after rejected oversized upload", asset.ThumbPath)
	}
}
