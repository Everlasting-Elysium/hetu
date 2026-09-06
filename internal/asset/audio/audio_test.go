package audio

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
	for _, ext := range []string{"mp3", "flac", "wav", "aac", "ogg", "m4a", "wma", "opus", "aiff"} {
		if !h.Match(ext) {
			t.Errorf("Match(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{"mp4", "jpg", "pdf", "txt", ""} {
		if h.Match(ext) {
			t.Errorf("Match(%q) = true, want false", ext)
		}
	}
}

func TestKind(t *testing.T) {
	if got := discardHandler().Kind(); got != domain.KindAudio {
		t.Fatalf("Kind() = %q, want %q", got, domain.KindAudio)
	}
}

func TestParseProbe(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantDur    time.Duration
		wantBR     int
		wantSR     int
		wantCh     int
		wantCodec  string
		wantErr    bool
	}{
		{
			name:      "full mp3",
			in:        `{"streams":[{"codec_name":"mp3","sample_rate":"44100","channels":2,"bit_rate":"320000"}],"format":{"duration":"180.5","bit_rate":"320000"}}`,
			wantDur:   180500 * time.Millisecond,
			wantBR:    320000,
			wantSR:    44100,
			wantCh:    2,
			wantCodec: "mp3",
		},
		{
			name:      "stream bitrate missing, falls back to format",
			in:        `{"streams":[{"codec_name":"flac","sample_rate":"96000","channels":2,"bit_rate":""}],"format":{"duration":"60.0","bit_rate":"1411200"}}`,
			wantDur:   60 * time.Second,
			wantBR:    1411200,
			wantSR:    96000,
			wantCh:    2,
			wantCodec: "flac",
		},
		{
			name: "empty streams",
			in:   `{"streams":[],"format":{"duration":"10.0","bit_rate":"128000"}}`,
			wantDur: 10 * time.Second,
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
			if pr.duration != tc.wantDur {
				t.Errorf("duration = %v, want %v", pr.duration, tc.wantDur)
			}
			if pr.bitrate != tc.wantBR {
				t.Errorf("bitrate = %d, want %d", pr.bitrate, tc.wantBR)
			}
			if pr.sampleRate != tc.wantSR {
				t.Errorf("sampleRate = %d, want %d", pr.sampleRate, tc.wantSR)
			}
			if pr.channels != tc.wantCh {
				t.Errorf("channels = %d, want %d", pr.channels, tc.wantCh)
			}
			if pr.codec != tc.wantCodec {
				t.Errorf("codec = %q, want %q", pr.codec, tc.wantCodec)
			}
		})
	}
}

func TestExtractReturnsAudioKind(t *testing.T) {
	h := discardHandler()
	meta, err := h.Extract(context.Background(), bytes.NewReader([]byte("not real audio")))
	if err != nil {
		t.Fatalf("Extract must not fail: %v", err)
	}
	if meta.Kind != domain.KindAudio {
		t.Fatalf("Kind = %q, want %q", meta.Kind, domain.KindAudio)
	}
	if meta.Width != 0 || meta.Height != 0 {
		t.Fatalf("dims = %dx%d, want 0x0 for audio", meta.Width, meta.Height)
	}
}

func TestThumbnailDegradesWithoutTools(t *testing.T) {
	h := discardHandler()
	err := h.Thumbnail(context.Background(), bytes.NewReader([]byte("x")), io.Discard)
	if err != domain.ErrNoThumbnail {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

func TestExtractMetadataDegradesWithoutTools(t *testing.T) {
	h := discardHandler()
	md, err := h.ExtractMetadata(context.Background(), bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatalf("ExtractMetadata must not fail: %v", err)
	}
	if len(md.Annotations) != 0 {
		t.Fatalf("annotations = %v, want empty when degraded", md.Annotations)
	}
}

// TestWithFFmpeg exercises the real subprocess path. Skipped when tools missing.
func TestWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	// Generate a 2-second sine-wave WAV.
	sample := filepath.Join(t.TempDir(), "sample.wav")
	gen := exec.Command("ffmpeg", "-nostdin", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=44100",
		sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate sample: %v\n%s", err, out)
	}

	h := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	f, err := os.Open(sample)
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()

	// Thumbnail: expect valid PNG (starts with PNG magic bytes).
	var buf bytes.Buffer
	if err := h.Thumbnail(context.Background(), f, &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	jpegMagic := []byte{0xFF, 0xD8, 0xFF}
	if !bytes.HasPrefix(buf.Bytes(), jpegMagic) {
		t.Fatalf("thumbnail is not JPEG (len=%d, prefix=%x)", buf.Len(), buf.Bytes()[:min(4, buf.Len())])
	}

	// ExtractMetadata: expect duration ~2s and codec info.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	md, err := h.ExtractMetadata(context.Background(), f)
	if err != nil {
		t.Fatalf("ExtractMetadata: %v", err)
	}
	dur, ok := md.Annotations[domain.KeyAudioDuration]
	if !ok {
		t.Fatal("missing audio.duration annotation")
	}
	durSec, ok := dur.(float64)
	if !ok || durSec < 1.5 || durSec > 2.5 {
		t.Fatalf("duration = %v, want ~2.0s", dur)
	}
	if _, ok := md.Annotations[domain.KeyAudioSampleRate]; !ok {
		t.Fatal("missing audio.sample_rate annotation")
	}
	if _, ok := md.Annotations[domain.KeyAudioChannels]; !ok {
		t.Fatal("missing audio.channels annotation")
	}
}
