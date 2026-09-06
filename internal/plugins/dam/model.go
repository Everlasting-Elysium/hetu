package dam

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/asset/model3d"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// glbMediaType is the IANA media type for binary glTF (.glb).
const glbMediaType = "model/gltf-binary"

// glbMagic is the 4-byte header that opens every binary glTF file. Converted
// output is validated against it before caching so a sidecar that returns 200
// with an empty or corrupt body never poisons the cache.
var glbMagic = []byte("glTF")

// cacheKeyOf keys the GLB cache by content hash so re-scanned or moved files
// reuse one conversion and changed content gets a fresh entry. It falls back to
// the asset id only if a hash is somehow absent (the indexer always sets one).
func cacheKeyOf(a domain.Asset) string {
	if a.Hash != "" {
		return a.Hash
	}
	return a.ID.String()
}

// modelCacheControl caches served models for a day. GLB conversions are keyed by
// content hash so a changed file yields a new URL/entry; the value is safe to
// hold for a while but is not marked immutable (an asset id can be re-pointed).
const modelCacheControl = "public, max-age=86400"

// serveModel handles GET /assets/{id}/model: it returns a browser-loadable 3D
// model (glTF/GLB) for the interactive viewer (#51). Native glTF/GLB stream
// straight from storage with Range support; other supported formats
// (OBJ/FBX/STL/USD/PLY) are converted to GLB via the Blender sidecar and cached
// under ModelCacheDir keyed by content hash, so the costly conversion runs at
// most once per unique file. Opaque/native formats (e.g. .ztl/.zpr) are not
// indexed as models and return 404 so the UI falls back to the preview image.
func (p *Plugin) serveModel(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	asset, err := p.k.Store.GetAsset(r.Context(), p.owner, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	ext := strings.ToLower(strings.TrimPrefix(asset.Ext, "."))
	if !model3d.Supported(ext) {
		http.NotFound(w, r)
		return
	}
	provider, ok := p.k.Storage.Get(asset.Provider)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError,
			fmt.Errorf("storage provider %q not registered", asset.Provider))
		return
	}

	if model3d.WebFriendly(ext) {
		p.streamModel(w, r, provider, asset, ext)
		return
	}

	if p.k.BlenderAddr == "" {
		httpjson.WriteError(w, http.StatusServiceUnavailable,
			fmt.Errorf("3D conversion unavailable: set HETU_BLENDER_ADDR"))
		return
	}
	glbPath, err := p.ensureGLB(r.Context(), provider, asset, ext)
	if err != nil {
		p.k.Log.WarnContext(r.Context(), "model3d: glb conversion failed",
			slog.String("asset", asset.ID.String()), slog.String("ext", ext), slog.Any("err", err))
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("convert model: %w", err))
		return
	}
	f, err := os.Open(glbPath)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("open glb: %w", err))
		return
	}
	defer func() { _ = f.Close() }()
	modTime := time.Time{}
	if info, statErr := f.Stat(); statErr == nil {
		modTime = info.ModTime()
	}
	w.Header().Set("Content-Type", glbMediaType)
	w.Header().Set("Cache-Control", modelCacheControl)
	// The content-hash cache key is a natural strong ETag for conditional GETs.
	w.Header().Set("ETag", `"`+cacheKeyOf(asset)+`"`)
	http.ServeContent(w, r, filepath.Base(glbPath), modTime, f)
}

// streamModel streams a web-friendly (glTF/GLB) asset straight from storage with
// Range support, so <model-viewer> can stream large models progressively.
func (p *Plugin) streamModel(w http.ResponseWriter, r *http.Request, provider kernel.StorageProvider, asset domain.Asset, ext string) {
	info, err := provider.Stat(r.Context(), asset.StoragePath)
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, fmt.Errorf("file not found: %w", err))
		return
	}
	f, err := provider.Open(r.Context(), asset.StoragePath)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("open file: %w", err))
		return
	}
	defer func() { _ = f.Close() }()
	ct := glbMediaType
	if ext == "gltf" {
		ct = "model/gltf+json"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", modelCacheControl)
	http.ServeContent(w, r, filepath.Base(asset.StoragePath), info.ModTime, f)
}

// ensureGLB returns the path to a cached GLB conversion of asset, producing it
// via the Blender sidecar on first request. The cache is keyed by content hash
// (see cacheKeyOf). Concurrent requests for the same model are collapsed into one
// conversion via singleflight, and total concurrent conversions are bounded by
// convertSem, so a burst of viewer opens cannot flood the sidecar.
func (p *Plugin) ensureGLB(ctx context.Context, provider kernel.StorageProvider, asset domain.Asset, ext string) (string, error) {
	key := cacheKeyOf(asset)
	glbPath := filepath.Join(p.k.ModelCacheDir, key+".glb")
	if _, err := os.Stat(glbPath); err == nil {
		return glbPath, nil
	}
	v, err, _ := p.convertGroup.Do(key, func() (any, error) {
		// A sibling flight may have finished the conversion while we queued.
		if _, statErr := os.Stat(glbPath); statErr == nil {
			return glbPath, nil
		}
		select {
		case p.convertSem <- struct{}{}:
			defer func() { <-p.convertSem }()
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if convErr := p.convertToCache(ctx, provider, asset, ext, glbPath); convErr != nil {
			return "", convErr
		}
		return glbPath, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// convertToCache converts asset to GLB via the Blender sidecar and commits it to
// glbPath atomically (temp file + rename). The output is validated as a real GLB
// before the rename, so a sidecar that returns 200 with an empty or corrupt body
// never lands in the cache.
func (p *Plugin) convertToCache(ctx context.Context, provider kernel.StorageProvider, asset domain.Asset, ext, glbPath string) error {
	if err := os.MkdirAll(p.k.ModelCacheDir, 0o755); err != nil {
		return fmt.Errorf("create model cache dir: %w", err)
	}
	src, err := provider.Open(ctx, asset.StoragePath)
	if err != nil {
		return fmt.Errorf("open source model: %w", err)
	}
	defer func() { _ = src.Close() }()

	tmp, err := os.CreateTemp(p.k.ModelCacheDir, "convert-*.glb.tmp")
	if err != nil {
		return fmt.Errorf("create temp glb: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once renamed

	convErr := model3d.ConvertToGLB(ctx, p.k.BlenderAddr, ext, src, tmp)
	closeErr := tmp.Close()
	if convErr != nil {
		return convErr
	}
	if closeErr != nil {
		return fmt.Errorf("close temp glb: %w", closeErr)
	}
	if err := validateGLB(tmpPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, glbPath); err != nil {
		return fmt.Errorf("commit glb cache: %w", err)
	}
	return nil
}

// validateGLB rejects an empty or non-GLB conversion result (bad or missing
// glbMagic) so a broken sidecar response is never cached.
func validateGLB(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("reopen converted glb: %w", err)
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, len(glbMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		return fmt.Errorf("converted glb too small: %w", err)
	}
	if !bytes.Equal(head, glbMagic) {
		return errors.New("converted output is not a GLB (bad magic)")
	}
	return nil
}
