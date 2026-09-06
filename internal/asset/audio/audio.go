// Package audio implements kernel.AssetHandler for audio files. It shells out
// to ffprobe (duration, bitrate, codec) and ffmpeg (waveform thumbnail via the
// showwavespic filter), keeping the kernel CGO-free. When those tools are absent
// the handler degrades gracefully: assets are still indexed as kind=audio, just
// without metadata or a thumbnail.
package audio

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
	waveWidth    = 800 // waveform thumbnail width in px
	waveHeight   = 200 // waveform thumbnail height in px
	probeTimeout = 30 * time.Second
	thumbTimeout = 60 * time.Second
)

var supported = map[string]struct{}{
	"mp3": {}, "flac": {}, "wav": {}, "aac": {}, "ogg": {}, "m4a": {},
	"wma": {}, "opus": {}, "aiff": {},
}

// Handler processes audio files via ffprobe/ffmpeg subprocesses.
type Handler struct {
	ffmpeg  string // resolved PATH, "" if missing
	ffprobe string
	log     *slog.Logger
	warn    sync.Once
}

var (
	_ kernel.AssetHandler      = (*Handler)(nil)
	_ kernel.MetadataExtractor = (*Handler)(nil)
)

// New returns an audio handler, resolving ffmpeg/ffprobe on PATH once.
func New(log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	ffprobe, _ := exec.LookPath("ffprobe")
	return &Handler{ffmpeg: ffmpeg, ffprobe: ffprobe, log: log}
}

func (h *Handler) Match(ext string) bool {
	_, ok := supported[ext]
	return ok
}

func (h *Handler) Kind() domain.AssetKind { return domain.KindAudio }

func (h *Handler) warnMissing(ctx context.Context) {
	h.warn.Do(func() {
		h.log.WarnContext(ctx,
			"ffmpeg/ffprobe not found on PATH; audio metadata and waveform thumbnails disabled",
			slog.Bool("ffmpeg", h.ffmpeg != ""), slog.Bool("ffprobe", h.ffprobe != ""))
	})
}

// Extract returns kind=audio with zero dimensions. Audio has no visual size.
func (h *Handler) Extract(_ context.Context, _ io.ReadSeeker) (domain.Meta, error) {
	return domain.Meta{Kind: domain.KindAudio}, nil
}

// Thumbnail renders a waveform JPEG into w using ffmpeg's showwavespic filter.
func (h *Handler) Thumbnail(ctx context.Context, src io.ReadSeeker, w io.Writer) error {
	if h.ffmpeg == "" {
		h.warnMissing(ctx)
		return domain.ErrNoThumbnail
	}
	path, cleanup, err := mediaproc.TempCopy(src, ".audio")
	if err != nil {
		return domain.ErrNoThumbnail
	}
	defer cleanup()

	filter := fmt.Sprintf("showwavespic=s=%dx%d:colors=#4f8ff7", waveWidth, waveHeight)
	out, err := mediaproc.Run(ctx, thumbTimeout, h.ffmpeg,
		"-nostdin", "-i", path,
		"-filter_complex", filter,
		"-frames:v", "1",
		"-f", "image2", "-vcodec", "mjpeg", "pipe:1")
	if err != nil {
		h.log.DebugContext(ctx, "audio waveform failed", slog.Any("err", err))
		return domain.ErrNoThumbnail
	}
	if len(out) == 0 {
		return domain.ErrNoThumbnail
	}
	if _, err := w.Write(out); err != nil {
		return fmt.Errorf("write waveform: %w", err)
	}
	return nil
}

// ExtractMetadata probes audio streams for duration, bitrate, sample_rate,
// channels, and codec, returning them as extracted-layer annotations.
func (h *Handler) ExtractMetadata(ctx context.Context, src io.ReadSeeker) (domain.ExtractedMetadata, error) {
	md := domain.ExtractedMetadata{Annotations: make(map[string]any)}
	if h.ffprobe == "" {
		h.warnMissing(ctx)
		return md, nil
	}
	path, cleanup, err := mediaproc.TempCopy(src, ".audio")
	if err != nil {
		return md, nil
	}
	defer cleanup()

	pr, err := h.probe(ctx, path)
	if err != nil {
		h.log.DebugContext(ctx, "audio probe failed", slog.Any("err", err))
		return md, nil
	}
	if pr.duration > 0 {
		md.Annotations[domain.KeyAudioDuration] = pr.duration.Seconds()
	}
	if pr.bitrate > 0 {
		md.Annotations[domain.KeyAudioBitrate] = pr.bitrate
	}
	if pr.sampleRate > 0 {
		md.Annotations[domain.KeyAudioSampleRate] = pr.sampleRate
	}
	if pr.channels > 0 {
		md.Annotations[domain.KeyAudioChannels] = pr.channels
	}
	if pr.codec != "" {
		md.Annotations[domain.KeyAudioCodec] = pr.codec
	}
	return md, nil
}

type probeResult struct {
	duration   time.Duration
	bitrate    int
	sampleRate int
	channels   int
	codec      string
}

func (h *Handler) probe(ctx context.Context, path string) (probeResult, error) {
	out, err := mediaproc.Run(ctx, probeTimeout, h.ffprobe,
		"-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name,sample_rate,channels,bit_rate:format=duration,bit_rate",
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
			CodecName  string `json:"codec_name"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			BitRate    string `json:"bit_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return probeResult{}, fmt.Errorf("parse ffprobe json: %w", err)
	}
	var pr probeResult
	if secs, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil && secs > 0 {
		pr.duration = time.Duration(secs * float64(time.Second))
	}
	if len(out.Streams) > 0 {
		s := out.Streams[0]
		pr.codec = s.CodecName
		pr.channels = s.Channels
		if rate, err := strconv.Atoi(s.SampleRate); err == nil {
			pr.sampleRate = rate
		}
		// Prefer stream bitrate; fall back to format bitrate.
		if br, err := strconv.Atoi(s.BitRate); err == nil {
			pr.bitrate = br
		} else if br, err := strconv.Atoi(out.Format.BitRate); err == nil {
			pr.bitrate = br
		}
	}
	return pr, nil
}
