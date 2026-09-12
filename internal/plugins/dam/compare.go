package dam

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/imgcompare"
)

const (
	// maxCompareUpload caps ONE side's uploaded image at 30 MiB. A comparison
	// input may be an original photo/scan (临摹/仿拍 originals), not a downscaled
	// preview, so the ceiling is higher than the 10 MiB client-thumbnail cap
	// (maxThumbUpload). Applied to both uploads and library-asset reads.
	maxCompareUpload = 30 << 20
	// maxCompareBody bounds the whole multipart body: both sides may upload, plus
	// the small text fields and multipart framing overhead.
	maxCompareBody = 2*maxCompareUpload + (1 << 20)
	// compareParseMemory is the in-memory multipart buffer; larger parts spill to
	// temp files, still bounded overall by maxCompareBody.
	compareParseMemory = 32 << 20
)

// sideReference and sideTarget are the two comparison sides. They double as the
// staged filename stem (<CompareDir>/<reqID>/<side>.<ext>) and the overlay path
// segment, so the same constant flows request -> disk -> response URL.
const (
	sideReference = "reference"
	sideTarget    = "target"
)

// compareResponse is the POST /compare result: the dimensions actually scored,
// the aggregate, each dimension's optional detail, the overlay URLs, and a
// critique placeholder. Dimensions that could not be computed (e.g. action/
// element when no Tagger is configured) are omitted rather than reported as a
// misleading perfect score, and excluded from Dimensions and OverallScore.
type compareResponse struct {
	ReqID        string                     `json:"req_id"`
	Dimensions   []string                   `json:"dimensions"`
	OverallScore float64                    `json:"overall_score"`
	Color        *imgcompare.ColorResult    `json:"color,omitempty"`
	Tone         *imgcompare.ToneResult     `json:"tone,omitempty"`
	Lighting     *imgcompare.LightingResult `json:"lighting,omitempty"`
	Action       *imgcompare.ActionResult   `json:"action,omitempty"`
	Element      *imgcompare.ElementResult  `json:"element,omitempty"`
	Overlays     compareOverlays            `json:"overlays"`
	Critique     compareCritique            `json:"critique"`
}

// compareOverlays carries the two overlay endpoints for the staged, normalized
// inputs so the frontend can render an aligned before/after view.
type compareOverlays struct {
	ReferenceURL string `json:"reference_url"`
	TargetURL    string `json:"target_url"`
}

// compareCritique is populated by a later PR (kernel.VisionCritic); this PR
// always reports it unavailable so the response shape is already stable for the
// frontend to integrate against.
type compareCritique struct {
	Available bool `json:"available"`
}

// compareAssets handles POST /api/dam/compare. Each side (reference/target) is
// either a library asset (<side>_asset_id) or a transient multipart upload
// (<side>_file); any cross-side combination is accepted. Optional ?dimensions=
// (comma-separated) narrows the five default axes. Both inputs are staged under
// a one-shot reqID for the overlay endpoint; nothing is ever indexed.
func (p *Plugin) compareAssets(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCompareBody)
	if err := r.ParseMultipartForm(compareParseMemory); err != nil {
		writeCompareParseErr(w, err)
		return
	}
	selected := parseDimensions(r.FormValue("dimensions"))

	ref, status, err := p.resolveSide(r, sideReference)
	if err != nil {
		httpjson.WriteError(w, status, err)
		return
	}
	tgt, status, err := p.resolveSide(r, sideTarget)
	if err != nil {
		httpjson.WriteError(w, status, err)
		return
	}

	reqID, err := newID()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	// Opportunistic sweep: reap stale staged dirs on the way in, so cleanup rides
	// normal traffic instead of a long-lived goroutine/ticker (see sweepCompareDir).
	p.sweepCompareDir(r.Context())

	refPath, err := p.stageCompareImage(reqID, sideReference, ref)
	if err != nil {
		p.cleanupCompareDir(reqID)
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	tgtPath, err := p.stageCompareImage(reqID, sideTarget, tgt)
	if err != nil {
		p.cleanupCompareDir(reqID)
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	// Uploads score against their staged copy; library assets against their
	// canonical storage path (the same ref the async embed/tag jobs use).
	if ref.assetRef == "" {
		ref.assetRef = refPath
	}
	if tgt.assetRef == "" {
		tgt.assetRef = tgtPath
	}

	resp := p.computeCompare(r.Context(), compareRequest{
		reqID: reqID, selected: selected, ref: ref, tgt: tgt,
	})
	httpjson.WriteJSON(w, http.StatusOK, resp)
}

// compareOverlay handles GET /api/dam/compare/{reqID}/overlay/{side}: it streams
// the staged, normalized bytes for one side of a prior comparison. An invalid
// side is 400; an unknown reqID or missing staged file is 404.
func (p *Plugin) compareOverlay(w http.ResponseWriter, r *http.Request) {
	side := chi.URLParam(r, "side")
	if side != sideReference && side != sideTarget {
		httpjson.WriteError(w, http.StatusBadRequest,
			fmt.Errorf("invalid side %q: want %q or %q", side, sideReference, sideTarget))
		return
	}
	reqID := chi.URLParam(r, "reqID")
	// reqID is a server-minted UUIDv7; parsing it as a UUID both rejects unknown
	// ids and blocks path traversal — a valid UUID has no '/' or '.' to escape
	// CompareDir once joined below.
	if _, err := uuid.Parse(reqID); err != nil {
		http.NotFound(w, r)
		return
	}
	matches, _ := filepath.Glob(filepath.Join(p.k.CompareDir, reqID, side+".*"))
	if len(matches) == 0 {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(matches[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if ct := contentType(strings.TrimPrefix(filepath.Ext(matches[0]), ".")); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// The staged bytes are immutable for the life of the one-shot reqID, so a
	// long cache is safe; the dir is TTL-swept, never rewritten in place.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, filepath.Base(matches[0]), info.ModTime(), f)
}

// overlayURL builds the absolute overlay path for a side. It derives the mount
// prefix from the plugin Name so it stays correct if the mount point changes.
func (p *Plugin) overlayURL(reqID, side string) string {
	return "/api/" + Name + "/compare/" + reqID + "/overlay/" + side
}

// writeCompareParseErr maps a multipart parse failure: an exceeded body cap is a
// 413 (the client sent too much), any other parse failure is a 400.
func writeCompareParseErr(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		httpjson.WriteError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("parse multipart form: %w", err))
}
