package model3d

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestHandler_Match(t *testing.T) {
	h := New("")
	for _, ext := range []string{"obj", "fbx", "glb", "gltf", "stl", "usd", "usdz", "ply"} {
		if !h.Match(ext) {
			t.Errorf("Match(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{"txt", "mp4"} {
		if h.Match(ext) {
			t.Errorf("Match(%q) = true, want false", ext)
		}
	}
}

func TestHandler_Kind(t *testing.T) {
	if got := New("").Kind(); got != domain.KindModel {
		t.Errorf("Kind() = %q, want %q", got, domain.KindModel)
	}
}

func TestHandler_Extract(t *testing.T) {
	meta, err := New("").Extract(context.Background(), strings.NewReader(""))
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if meta.Kind != domain.KindModel {
		t.Errorf("Kind = %q, want %q", meta.Kind, domain.KindModel)
	}
	if meta.Width != 0 || meta.Height != 0 {
		t.Errorf("dims = %dx%d, want 0x0", meta.Width, meta.Height)
	}
}

func TestHandler_ThumbnailNoSidecar(t *testing.T) {
	err := New("").Thumbnail(context.Background(), strings.NewReader("x"), io.Discard)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Thumbnail() error = %v, want ErrNoThumbnail", err)
	}
}
