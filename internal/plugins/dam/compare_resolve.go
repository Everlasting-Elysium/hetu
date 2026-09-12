package dam

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	// Register the standard-library decoders so image.Decode handles the formats
	// hetu compares. Uploads are restricted to png/jpeg by ensureImagePNGorJPEG;
	// gif covers library assets that decode as a single frame.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// sideData is one resolved comparison side: the decoded image for the pixel
// dimensions, the raw bytes to stage for overlay, its file extension, a ref
// string for the Tagger/Embedder (a library asset's storage path, or — filled in
// after staging — the staged upload path), and any pre-existing CLIP embedding
// reused from a library asset (nil for uploads).
type sideData struct {
	img       image.Image
	raw       []byte
	ext       string
	assetRef  string
	embedding []float32
}

// resolveSide reads one side from the multipart form: either <side>_asset_id (a
// library asset) or <side>_file (a transient upload). It returns the resolved
// side plus an HTTP status/error. Supplying both, or neither, is a 400.
func (p *Plugin) resolveSide(r *http.Request, side string) (sideData, int, error) {
	assetID := strings.TrimSpace(r.FormValue(side + "_asset_id"))
	file, _, ferr := r.FormFile(side + "_file")
	switch {
	case assetID != "" && ferr == nil:
		_ = file.Close()
		return sideData{}, http.StatusBadRequest,
			fmt.Errorf("%s: provide either %s_asset_id or %s_file, not both", side, side, side)
	case assetID != "":
		return p.resolveLibrarySide(r.Context(), assetID)
	case ferr == nil:
		defer func() { _ = file.Close() }()
		return resolveUploadSide(file)
	default:
		return sideData{}, http.StatusBadRequest,
			fmt.Errorf("%s: provide %s_asset_id or %s_file", side, side, side)
	}
}

// resolveLibrarySide loads a library asset's bytes and decodes them. A malformed
// id or an undecodable asset is a 400, an unknown id is a 404, storage failures
// are 500. Any stored CLIP embedding is reused so the element dimension need not
// re-embed a library asset.
func (p *Plugin) resolveLibrarySide(ctx context.Context, rawID string) (sideData, int, error) {
	id, err := domain.NewAssetID(rawID)
	if err != nil {
		return sideData{}, http.StatusBadRequest, err
	}
	asset, err := p.k.Store.GetAsset(ctx, p.owner, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return sideData{}, http.StatusNotFound, err
		}
		return sideData{}, http.StatusInternalServerError, err
	}
	raw, err := p.readAssetBytes(ctx, asset)
	if err != nil {
		return sideData{}, http.StatusInternalServerError, err
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return sideData{}, http.StatusBadRequest, fmt.Errorf("decode asset %s: %w", id, err)
	}
	ext := asset.Ext
	if ext == "" {
		ext = extFromFormat(format)
	}
	side := sideData{img: img, raw: raw, ext: ext, assetRef: asset.StoragePath}
	if emb, embErr := p.k.Store.GetEmbedding(ctx, id); embErr == nil {
		side.embedding = emb
	}
	return side, http.StatusOK, nil
}

// resolveUploadSide validates and decodes a multipart upload. Non-PNG/JPEG
// bodies are rejected by magic-byte sniff (reusing ensureImagePNGorJPEG), an
// oversized body is a 413, an undecodable body is a 400. assetRef stays empty
// and is filled with the staged path after staging.
func resolveUploadSide(file multipart.File) (sideData, int, error) {
	if err := ensureImagePNGorJPEG(file); err != nil {
		return sideData{}, http.StatusBadRequest, err
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxCompareUpload+1))
	if err != nil {
		return sideData{}, http.StatusBadRequest, fmt.Errorf("read upload: %w", err)
	}
	if len(raw) > maxCompareUpload {
		return sideData{}, http.StatusRequestEntityTooLarge,
			fmt.Errorf("upload exceeds %d bytes", maxCompareUpload)
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return sideData{}, http.StatusBadRequest, fmt.Errorf("decode upload: %w", err)
	}
	return sideData{img: img, raw: raw, ext: extFromFormat(format)}, http.StatusOK, nil
}

// readAssetBytes opens an asset through its storage provider and reads up to
// maxCompareUpload bytes from the anchor storage path (the same ref the embed/
// tag pipeline uses); comparison scores the indexed image, not a version.
func (p *Plugin) readAssetBytes(ctx context.Context, asset domain.Asset) ([]byte, error) {
	provider, ok := p.k.Storage.Get(asset.Provider)
	if !ok {
		return nil, fmt.Errorf("storage provider %q not registered", asset.Provider)
	}
	f, err := provider.Open(ctx, asset.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("open asset: %w", err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxCompareUpload+1))
	if err != nil {
		return nil, fmt.Errorf("read asset: %w", err)
	}
	if len(raw) > maxCompareUpload {
		return nil, fmt.Errorf("asset exceeds %d bytes", maxCompareUpload)
	}
	return raw, nil
}

// extFromFormat maps an image.Decode format name to a staged-file extension.
func extFromFormat(format string) string {
	switch format {
	case "jpeg":
		return "jpg"
	case "gif":
		return "gif"
	default:
		return "png"
	}
}
