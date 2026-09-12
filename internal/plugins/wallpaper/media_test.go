package wallpaper

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestServeThumb covers /thumb: streams the thumbnail bytes when present, 404
// when the asset has no thumbnail, and 404 for an unknown asset.
func TestServeThumb(t *testing.T) {
	e := newTestEnv(t)
	withThumb := e.add(seed{id: "a1", name: "one.png", thumb: []byte("THUMBDATA")})
	e.add(seed{id: "a2", name: "two.png"})

	resp := e.get("/api/wallpaper/" + withThumb.String() + "/thumb")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("thumb = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "THUMBDATA" {
		t.Errorf("thumb body = %q, want THUMBDATA", body)
	}

	if r := e.get("/api/wallpaper/a2/thumb"); r.StatusCode != http.StatusNotFound {
		_ = r.Body.Close()
		t.Errorf("no-thumb asset = %d, want 404", r.StatusCode)
	}
	if r := e.get("/api/wallpaper/does-not-exist/thumb"); r.StatusCode != http.StatusNotFound {
		_ = r.Body.Close()
		t.Errorf("missing asset thumb = %d, want 404", r.StatusCode)
	}
}

// TestDownloadAttachment covers /download: it forces a save (Content-Disposition
// attachment, unlike DAM's inline /file), streams the original bytes, and 404s
// for an unknown asset.
func TestDownloadAttachment(t *testing.T) {
	e := newTestEnv(t)
	a1 := e.add(seed{id: "a1", name: "wall.png", content: []byte("IMAGEBYTES")})

	resp := e.get("/api/wallpaper/" + a1.String() + "/download")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download = %d, want 200", resp.StatusCode)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("Content-Disposition = %q, want attachment", cd)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "IMAGEBYTES" {
		t.Errorf("download body = %q, want IMAGEBYTES", body)
	}

	if r := e.get("/api/wallpaper/does-not-exist/download"); r.StatusCode != http.StatusNotFound {
		_ = r.Body.Close()
		t.Errorf("missing asset download = %d, want 404", r.StatusCode)
	}
}

// TestDownloadUsesDisplayName covers the download filename falling back to the
// user-facing display name when set.
func TestDownloadUsesDisplayName(t *testing.T) {
	e := newTestEnv(t)
	a1 := e.add(seed{id: "a1", name: "raw-file.png", content: []byte("X"), display: "Sunset.png"})

	resp := e.get("/api/wallpaper/" + a1.String() + "/download")
	defer func() { _ = resp.Body.Close() }()
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, `"Sunset.png"`) {
		t.Errorf("Content-Disposition = %q, want display name Sunset.png", cd)
	}
}
