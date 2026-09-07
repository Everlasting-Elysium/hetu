package dam

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

const (
	// frameTimeout bounds a single frame extraction (seek + decode + encode).
	frameTimeout = 30 * time.Second
	// frameMaxDim caps the longest edge (px) of the returned JPEG, matching the
	// scrub-preview use case; a full-resolution frame is never needed here.
	frameMaxDim = 1024
)

// extractFrame handles GET /assets/{id}/frame?ms=<t>: it decodes a single frame
// of a video asset at the given millisecond offset and returns it as JPEG. Only
// kind=video assets are valid; a missing or non-numeric ms, or a non-video
// asset, is a 400. The frame is deterministic for a given (asset, ms), so it is
// served with a long immutable cache. ffmpeg does the decode, mirroring
// video.Handler.Thumbnail; when ffmpeg is absent the request fails with 500.
func (p *Plugin) extractFrame(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	ms, err := frameMillis(r)
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
	if asset.Kind != domain.KindVideo {
		httpjson.WriteError(w, http.StatusBadRequest,
			fmt.Errorf("asset %s is not a video", id))
		return
	}

	path, cleanup, err := p.videoTempFile(r.Context(), asset)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	defer cleanup()

	out, err := runFrameFFmpeg(r.Context(), path, ms)
	if err != nil {
		p.k.Log.DebugContext(r.Context(), "extract frame failed",
			slog.String("asset", id.String()), slog.Int64("ms", ms), slog.Any("err", err))
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	if _, err := w.Write(out); err != nil {
		p.k.Log.DebugContext(r.Context(), "write frame failed", slog.Any("err", err))
	}
}

// frameMillis reads the required ms query param as a non-negative integer.
func frameMillis(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("ms")
	if raw == "" {
		return 0, errors.New("query param ms is required")
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms < 0 {
		return 0, fmt.Errorf("invalid ms %q: want a non-negative integer", raw)
	}
	return ms, nil
}

// videoTempFile resolves the asset's current-version bytes through its storage
// provider and copies them to a temp file, returning the path plus a cleanup the
// caller must defer. ffmpeg needs a seekable path, not a stream, and the
// provider abstraction hides the on-disk path, so the copy keeps the endpoint
// storage-backend agnostic (mirroring video.Handler.Thumbnail).
func (p *Plugin) videoTempFile(ctx context.Context, asset domain.Asset) (string, func(), error) {
	providerName, storagePath := asset.Provider, asset.StoragePath
	if v, ok := p.currentVersionFile(ctx, asset); ok {
		providerName, storagePath = v.Provider, v.StoragePath
	}
	provider, ok := p.k.Storage.Get(providerName)
	if !ok {
		return "", nil, fmt.Errorf("storage provider %q not registered", providerName)
	}
	f, err := provider.Open(ctx, storagePath)
	if err != nil {
		return "", nil, fmt.Errorf("open asset: %w", err)
	}
	defer func() { _ = f.Close() }()
	return mediaproc.TempCopy(f, ".video")
}

// runFrameFFmpeg decodes the frame at ms (converted to a fractional-second seek)
// and returns the JPEG bytes. -ss before -i does a fast input seek; the scale
// filter bounds the longest edge to frameMaxDim while preserving aspect ratio.
func runFrameFFmpeg(ctx context.Context, path string, ms int64) ([]byte, error) {
	scale := fmt.Sprintf(
		"scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease",
		frameMaxDim, frameMaxDim)
	seek := strconv.FormatFloat(float64(ms)/1000.0, 'f', 3, 64)
	out, err := mediaproc.Run(ctx, frameTimeout, "ffmpeg",
		"-nostdin", "-ss", seek, "-i", path,
		"-frames:v", "1", "-vf", scale,
		"-f", "image2", "-vcodec", "mjpeg", "pipe:1")
	if err != nil {
		return nil, fmt.Errorf("extract frame: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ffmpeg produced no frame at ms=%d", ms)
	}
	return out, nil
}
