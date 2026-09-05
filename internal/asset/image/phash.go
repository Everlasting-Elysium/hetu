package image

import (
	"context"
	"fmt"
	stdimage "image"
	"io"

	"github.com/corona10/goimagehash"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

var _ kernel.PHashExtractor = (*Handler)(nil)

// PHash computes a 64-bit perceptual hash of the image. The caller is
// responsible for opening and closing the reader; PHash does a full decode.
func (h *Handler) PHash(_ context.Context, src io.ReadSeeker) (uint64, error) {
	img, _, err := stdimage.Decode(src)
	if err != nil {
		return 0, fmt.Errorf("decode image for phash: %w", err)
	}
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return 0, fmt.Errorf("compute phash: %w", err)
	}
	return hash.GetHash(), nil
}
