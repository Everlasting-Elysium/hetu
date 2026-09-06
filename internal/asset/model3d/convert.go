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

// ConvertToGLB POSTs src to the Blender sidecar's /convert endpoint as a
// multipart body and copies the returned GLB into w. ext (lowercase, no dot) is
// the model's true format, passed to the sidecar so it selects the right Blender
// importer instead of guessing from magic bytes — essential for binary STL,
// which carries no reliable signature. Unlike Thumbnail, a conversion failure is
// returned verbatim rather than degraded to a sentinel: the caller is a
// user-initiated viewer request, so a real error must surface for the UI to fall
// back on — never a silent empty model. An empty blenderAddr is a programming
// error (callers gate on it) and returns an error immediately.
func ConvertToGLB(ctx context.Context, blenderAddr, ext string, src io.ReadSeeker, w io.Writer) error {
	if blenderAddr == "" {
		return fmt.Errorf("model3d: no blender sidecar configured")
	}
	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()

	body, contentType, err := multipartBody(src)
	if err != nil {
		return fmt.Errorf("model3d: build convert request: %w", err)
	}
	reqURL := "http://" + blenderAddr + "/convert?ext=" + url.QueryEscape(ext)
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
