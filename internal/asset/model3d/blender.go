package model3d

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// renderTimeout bounds a single Blender render request. A 3D render is far
// slower than image decoding, so the deadline is generous but finite.
const renderTimeout = 60 * time.Second

// renderThumbnail POSTs src to the Blender sidecar's /render endpoint as a
// multipart body and copies the returned PNG into w. Any network, HTTP, or
// copy error degrades to domain.ErrNoThumbnail: a missing or failing sidecar
// must never block a scan.
func renderThumbnail(ctx context.Context, blenderAddr string, src io.ReadSeeker, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()

	body, contentType, err := multipartBody(src)
	if err != nil {
		return degrade(blenderAddr, err)
	}
	url := "http://" + blenderAddr + "/render"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return degrade(blenderAddr, err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return degrade(blenderAddr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return degrade(blenderAddr, fmt.Errorf("blender sidecar status %d", resp.StatusCode))
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return degrade(blenderAddr, err)
	}
	return nil
}

// multipartBody buffers src into a multipart form with a single "model" field.
// The sidecar sniffs the model format from the content, so no filename or
// extension is sent.
func multipartBody(src io.ReadSeeker) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("model", "model")
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(part, src); err != nil {
		return nil, "", err
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return &buf, mw.FormDataContentType(), nil
}

// degrade logs why a render failed and returns the graceful-degradation
// sentinel the indexer treats as "no thumbnail, keep scanning".
func degrade(blenderAddr string, err error) error {
	slog.Warn("model3d: blender render failed; degrading to no thumbnail",
		slog.String("addr", blenderAddr), slog.Any("err", err))
	return domain.ErrNoThumbnail
}
