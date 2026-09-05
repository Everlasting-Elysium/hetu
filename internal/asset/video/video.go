// Package video implements kernel.AssetHandler for video files. It shells out
// to ffprobe (dimensions + duration) and ffmpeg (a representative keyframe
// thumbnail), keeping the kernel CGO-free. When those tools are absent the
// handler degrades gracefully: assets are still indexed as kind=video, just
// without dimensions or a thumbnail.
package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

const (
	// thumbMaxDim is the longest edge (px) of a generated thumbnail.
	thumbMaxDim = 512
	// seekFraction picks a representative frame at this fraction of the
	// video's duration, skipping black intro frames.
	seekFraction = 0.1
	// seekFallback is used when the duration is unknown (probe failed).
	seekFallback = 1 * time.Second
	// seekCap bounds the seek so very long videos still thumbnail quickly.
	seekCap      = 10 * time.Second
	probeTimeout = 30 * time.Second
	thumbTimeout = 60 * time.Second
)

var supported = map[string]struct{}{
	"mp4": {}, "mov": {}, "mkv": {}, "webm": {}, "avi": {}, "m4v": {},
}

// Handler processes video files via ffprobe/ffmpeg subprocesses.
type Handler struct {
	ffmpeg  string // resolved PATH, "" if missing
	ffprobe string
	log     *slog.Logger
	warn    sync.Once
}

var _ kernel.AssetHandler = (*Handler)(nil)

// New returns a video handler, resolving ffmpeg/ffprobe on PATH once. A nil log
// falls back to slog.Default.
func New(log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	ffprobe, _ := exec.LookPath("ffprobe")
	return &Handler{ffmpeg: ffmpeg, ffprobe: ffprobe, log: log}
}

// Match reports whether ext is a supported video extension.
func (h *Handler) Match(ext string) bool {
	_, ok := supported[ext]
	return ok
}

// Kind returns domain.KindVideo.
func (h *Handler) Kind() domain.AssetKind { return domain.KindVideo }

func (h *Handler) available() bool { return h.ffmpeg != "" && h.ffprobe != "" }

func (h *Handler) warnMissing(ctx context.Context) {
	h.warn.Do(func() {
		h.log.WarnContext(ctx,
			"ffmpeg/ffprobe not found on PATH; video dimensions and thumbnails disabled",
			slog.Bool("ffmpeg", h.ffmpeg != ""), slog.Bool("ffprobe", h.ffprobe != ""))
	})
}

// Extract returns kind plus dimensions from ffprobe. It is best-effort: a
// missing tool or a probe failure yields kind=video with zero dimensions and a
// nil error so the scan never fails.
func (h *Handler) Extract(ctx context.Context, src io.ReadSeeker) (domain.Meta, error) {
	meta := domain.Meta{Kind: domain.KindVideo}
	if !h.available() {
		h.warnMissing(ctx)
		return meta, nil
	}
	path, cleanup, err := mediaproc.TempCopy(src, ".video")
	if err != nil {
		h.log.DebugContext(ctx, "video temp copy failed", slog.Any("err", err))
		return meta, nil
	}
	defer cleanup()
	pr, err := h.probe(ctx, path)
	if err != nil {
		h.log.DebugContext(ctx, "video probe failed", slog.Any("err", err))
		return meta, nil
	}
	meta.Width, meta.Height = pr.width, pr.height
	return meta, nil
}

// Thumbnail renders one representative keyframe as JPEG into w, or returns
// domain.ErrNoThumbnail if the tools are missing or ffmpeg fails.
func (h *Handler) Thumbnail(ctx context.Context, src io.ReadSeeker, w io.Writer) error {
	if !h.available() {
		h.warnMissing(ctx)
		return domain.ErrNoThumbnail
	}
	path, cleanup, err := mediaproc.TempCopy(src, ".video")
	if err != nil {
		h.log.DebugContext(ctx, "video temp copy failed", slog.Any("err", err))
		return domain.ErrNoThumbnail
	}
	defer cleanup()

	seek := seekFallback
	if pr, err := h.probe(ctx, path); err == nil && pr.duration > 0 {
		seek = min(time.Duration(float64(pr.duration)*seekFraction), seekCap)
	}
	scale := fmt.Sprintf(
		"scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease",
		thumbMaxDim, thumbMaxDim)
	out, err := mediaproc.Run(ctx, thumbTimeout, h.ffmpeg,
		"-nostdin", "-ss", formatSeek(seek), "-i", path,
		"-frames:v", "1", "-vf", scale,
		"-f", "image2", "-vcodec", "mjpeg", "pipe:1")
	if err != nil {
		h.log.DebugContext(ctx, "video thumbnail failed", slog.Any("err", err))
		return domain.ErrNoThumbnail
	}
	if len(out) == 0 {
		return domain.ErrNoThumbnail
	}
	if _, err := w.Write(out); err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}
	return nil
}

type probeResult struct {
	width, height int
	duration      time.Duration
}

func (h *Handler) probe(ctx context.Context, path string) (probeResult, error) {
	out, err := mediaproc.Run(ctx, probeTimeout, h.ffprobe,
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height:format=duration",
		"-of", "json", path)
	if err != nil {
		return probeResult{}, fmt.Errorf("ffprobe: %w", err)
	}
	return parseProbe(out)
}

// parseProbe turns ffprobe JSON into a probeResult. It is pure so it can be
// unit-tested without ffprobe installed.
func parseProbe(data []byte) (probeResult, error) {
	var out struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return probeResult{}, fmt.Errorf("parse ffprobe json: %w", err)
	}
	var pr probeResult
	if len(out.Streams) > 0 {
		pr.width, pr.height = out.Streams[0].Width, out.Streams[0].Height
	}
	if secs, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil && secs > 0 {
		pr.duration = time.Duration(secs * float64(time.Second))
	}
	return pr, nil
}

func formatSeek(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}
