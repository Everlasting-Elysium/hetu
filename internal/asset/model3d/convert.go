package model3d

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// convertTimeout bounds a single Blender GLB conversion end to end (body upload
// + Blender run + response). It is set above the sidecar's own 120s subprocess
// timeout so a slow Blender surfaces as the sidecar's real error rather than a
// generic client-side context cancellation.
const convertTimeout = 150 * time.Second

// BlenderConverter converts 3D models to GLB via the Blender headless sidecar's
// /convert HTTP endpoint. addr is the sidecar host:port; an empty addr is a
// programming error (the factory/callers gate on availability) and every
// conversion returns an error immediately.
type BlenderConverter struct{ addr string }

// ConvertToGLB POSTs src to the Blender sidecar's /convert endpoint as a
// multipart body and copies the returned GLB into w. ext (lowercase, no dot) is
// the model's true format, passed to the sidecar so it selects the right Blender
// importer instead of guessing from magic bytes — essential for binary STL,
// which carries no reliable signature. A conversion failure is returned verbatim
// rather than degraded to a sentinel: the caller is a user-initiated viewer
// request, so a real error must surface for the UI to fall back on — never a
// silent empty model.
func (b *BlenderConverter) ConvertToGLB(ctx context.Context, ext string, src io.ReadSeeker, w io.Writer) error {
	if b.addr == "" {
		return fmt.Errorf("model3d: no blender sidecar configured")
	}
	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()

	body, contentType, err := multipartBody(src)
	if err != nil {
		return fmt.Errorf("model3d: build convert request: %w", err)
	}
	reqURL := "http://" + b.addr + "/convert?ext=" + url.QueryEscape(ext)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return fmt.Errorf("model3d: new convert request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("model3d: convert request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("model3d: blender sidecar convert status %d", resp.StatusCode)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("model3d: copy converted glb: %w", err)
	}
	return nil
}

// ConvertToGLB is a deprecated package-level shim retained for backward
// compatibility. It delegates to a transient BlenderConverter; new callers
// should build a Converter via NewConverter and invoke the method directly.
//
// Deprecated: use NewConverter(...).ConvertToGLB or a *BlenderConverter.
func ConvertToGLB(ctx context.Context, blenderAddr, ext string, src io.ReadSeeker, w io.Writer) error {
	return (&BlenderConverter{addr: blenderAddr}).ConvertToGLB(ctx, ext, src, w)
}
