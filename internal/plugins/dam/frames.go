package dam

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// assetFrameDTO is the wire form of one frame of an image sequence (issue #62).
// Only the frame number and display name are exposed; the frame's bytes stream
// from /assets/{id}/frames/{n}, so the storage path never reaches the client.
type assetFrameDTO struct {
	FrameNo int    `json:"frame_no"`
	Name    string `json:"name"`
}

// listAssetFrames handles GET /assets/{id}/frames: the ordered frame list of an
// image sequence (issue #62), by frame number. Returns 200 with [] for an asset
// that is not a sequence (a plain single image, or any non-sequence asset), and
// 404 for a missing or cross-owner asset (via requireAsset). The frontend shows
// its step viewer only when this list has >= 2 frames.
func (p *Plugin) listAssetFrames(w http.ResponseWriter, r *http.Request) {
	a, ok := p.requireAsset(w, r)
	if !ok {
		return
	}
	frames, err := p.k.Store.ListAssetFrames(r.Context(), p.owner, a.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]assetFrameDTO, 0, len(frames))
	for _, fr := range frames {
		out = append(out, assetFrameDTO{FrameNo: fr.FrameNo, Name: fr.Name})
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// serveFrame handles GET /assets/{id}/frames/{frameNo}: it streams the original
// bytes of the n-th frame from the asset's storage provider, mirroring serveFile
// (frames carry no generated thumbnail — the stepper reads the source file on
// demand). Scoped to the plugin owner; the storage path is resolved server-side
// and never exposed. Returns 404 when the asset, frame, or file is absent, and
// 400 for a non-numeric or out-of-range frame number.
func (p *Plugin) serveFrame(w http.ResponseWriter, r *http.Request) {
	a, ok := p.requireAsset(w, r)
	if !ok {
		return
	}
	frameNo, err := strconv.Atoi(chi.URLParam(r, "frameNo"))
	if err != nil || frameNo < 1 {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("invalid frame number"))
		return
	}
	frames, err := p.k.Store.ListAssetFrames(r.Context(), p.owner, a.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	frame, ok := frameByNo(frames, frameNo)
	if !ok {
		http.NotFound(w, r)
		return
	}
	provider, ok := p.k.Storage.Get(a.Provider)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError,
			fmt.Errorf("storage provider %q not registered", a.Provider))
		return
	}
	info, err := provider.Stat(r.Context(), frame.StoragePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := provider.Open(r.Context(), frame.StoragePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(frame.StoragePath), "."))
	if ct := contentType(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, frame.Name, info.ModTime, f)
}

// frameByNo returns the frame with frameNo, or ok=false when absent.
func frameByNo(frames []domain.AssetFrame, frameNo int) (domain.AssetFrame, bool) {
	for _, fr := range frames {
		if fr.FrameNo == frameNo {
			return fr, true
		}
	}
	return domain.AssetFrame{}, false
}
