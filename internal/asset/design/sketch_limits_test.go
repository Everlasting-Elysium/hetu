package design

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// zeroReader yields an endless stream of zero bytes (a highly compressible input
// for building a zip bomb).
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// TestThumbnailSketchRejectsZipBomb asserts a preview entry that decompresses far
// beyond the cap is rejected via a bounded read — returning ErrNoThumbnail with
// no output — rather than expanding unbounded into memory or streaming out.
func TestThumbnailSketchRejectsZipBomb(t *testing.T) {
	data := sketchZipBomb(t, sketchPreviewPath, maxPreviewBytes+(1<<20))
	var buf bytes.Buffer
	err := New(nil).Thumbnail(context.Background(), bytes.NewReader(data), &buf)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %d bytes, want 0 (a bomb must not stream out)", buf.Len())
	}
}

// sketchZipBomb builds a Sketch bundle whose named entry decompresses to size
// bytes of zeros, streamed so the test itself never holds the expanded payload.
func sketchZipBomb(t *testing.T, name string, size int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := io.CopyN(f, zeroReader{}, size); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
