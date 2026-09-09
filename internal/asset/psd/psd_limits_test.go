package psd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// TestThumbnailRejectsOversizedDims asserts a PSD whose header declares
// dimensions past the decode limit is rejected from the header (before its pixel
// buffer is allocated by psd.Decode), even though its merged image is otherwise
// decodable. 25000 px wide is valid per PSD v1 (<=30000) but over thumb.MaxDim
// (20000), so the dimension guard must short-circuit to ErrNoThumbnail.
func TestThumbnailRejectsOversizedDims(t *testing.T) {
	err := New().Thumbnail(context.Background(), bytes.NewReader(flatPSD(25000, 1)), io.Discard)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}
