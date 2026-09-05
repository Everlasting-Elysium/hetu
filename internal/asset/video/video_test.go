package video

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func discardHandler() *Handler {
	return &Handler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestMatch(t *testing.T) {
	h := discardHandler()
	for _, ext := range []string{"mp4", "mov", "mkv", "webm", "avi", "m4v"} {
		if !h.Match(ext) {
			t.Errorf("Match(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{"jpg", "png", "pdf", "mp3", ""} {
		if h.Match(ext) {
			t.Errorf("Match(%q) = true, want false", ext)
		}
	}
}

func TestKind(t *testing.T) {
	if got := discardHandler().Kind(); got != domain.KindVideo {
		t.Fatalf("Kind() = %q, want %q", got, domain.KindVideo)
	}
}

func TestParseProbe(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		wantW, wantH int
		wantDur      time.Duration
		wantErr      bool
	}{
		{
			name:    "full",
			in:      `{"streams":[{"width":1920,"height":1080}],"format":{"duration":"12.500000"}}`,
			wantW:   1920,
			wantH:   1080,
			wantDur: 12500 * time.Millisecond,
		},
		{
			name:  "no duration",
			in:    `{"streams":[{"width":640,"height":480}],"format":{}}`,
			wantW: 640,
			wantH: 480,
		},
		{
			name:    "empty streams keeps duration",
			in:      `{"streams":[],"format":{"duration":"3.0"}}`,
			wantDur: 3 * time.Second,
		},
		{
			name:    "invalid json",
			in:      `not json`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pr, err := parseProbe([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pr.width != tc.wantW || pr.height != tc.wantH {
				t.Fatalf("dims = %dx%d, want %dx%d", pr.width, pr.height, tc.wantW, tc.wantH)
			}
			if pr.duration != tc.wantDur {
				t.Fatalf("duration = %v, want %v", pr.duration, tc.wantDur)
			}
		})
	}
}

func TestExtractDegradesWithoutTools(t *testing.T) {
	h := discardHandler() // ffmpeg/ffprobe empty -> unavailable
	meta, err := h.Extract(context.Background(), bytes.NewReader([]byte("not a real video")))
	if err != nil {
		t.Fatalf("Extract must not fail the scan: %v", err)
	}
	if meta.Kind != domain.KindVideo {
		t.Fatalf("Kind = %q, want %q", meta.Kind, domain.KindVideo)
	}
	if meta.Width != 0 || meta.Height != 0 {
		t.Fatalf("dims = %dx%d, want 0x0 when degraded", meta.Width, meta.Height)
	}
}

func TestThumbnailDegradesWithoutTools(t *testing.T) {
	h := discardHandler()
	err := h.Thumbnail(context.Background(), bytes.NewReader([]byte("x")), io.Discard)
	if err == nil || err != domain.ErrNoThumbnail {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

func TestFormatSeek(t *testing.T) {
	if got := formatSeek(1500 * time.Millisecond); got != "1.500" {
		t.Fatalf("formatSeek = %q, want %q", got, "1.500")
	}
}

// TestExtractAndThumbnailWithFFmpeg exercises the real subprocess path. It is
// skipped when ffmpeg/ffprobe are not installed so CI stays green.
func TestExtractAndThumbnailWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	sample := filepath.Join(t.TempDir(), "sample.mp4")
	gen := exec.Command("ffmpeg", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=128x96:rate=5", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate sample: %v\n%s", err, out)
	}

	h := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	f, err := os.Open(sample)
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()

	meta, err := h.Extract(context.Background(), f)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if meta.Width != 128 || meta.Height != 96 {
		t.Fatalf("dims = %dx%d, want 128x96", meta.Width, meta.Height)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	var buf bytes.Buffer
	if err := h.Thumbnail(context.Background(), f, &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("thumbnail is not JPEG (len=%d)", buf.Len())
	}
}
