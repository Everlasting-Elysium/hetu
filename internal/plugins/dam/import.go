package dam

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/importers"
	"github.com/Everlasting-Elysium/hetu/internal/importers/billfish"
	"github.com/Everlasting-Elysium/hetu/internal/importers/eagle"
)

// importReq is the JSON body for a path-based loose-file import (#18).
type importReq struct {
	Mode       string `json:"mode"`        // index|copy|move; empty = index
	Path       string `json:"path"`        // absolute source path
	DestSubdir string `json:"dest_subdir"` // copy/move destination subdir
	Conflict   string `json:"conflict"`    // keep-both|skip
}

// importResp reports a single-file import outcome.
type importResp struct {
	Asset   *assetDTO `json:"asset,omitempty"`
	Skipped bool      `json:"skipped"`
}

// importAsset handles POST /import: a JSON body imports a file by path; a
// multipart/form-data body imports an uploaded file (copied into the library).
func (p *Plugin) importAsset(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		p.importUpload(w, r)
		return
	}
	var req importReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Path == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("path required"))
		return
	}
	svc := importers.New(p.k, p.owner)
	asset, skipped, err := svc.ImportPath(r.Context(), req.Path, importers.Options{
		Mode: importers.Mode(req.Mode), DestSubdir: req.DestSubdir, Conflict: importers.Conflict(req.Conflict),
	})
	p.writeImportResult(w, asset, skipped, err)
}

// importUpload handles a multipart upload: the bytes are written to a temp file
// then copied into the library (uploads never index-in-place a temp path).
func (p *Plugin) importUpload(w http.ResponseWriter, r *http.Request) {
	file, hdr, err := r.FormFile("file")
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("form file: %w", err))
		return
	}
	defer func() { _ = file.Close() }()
	tmp, err := os.CreateTemp("", "hetu-upload-*")
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := io.Copy(tmp, file); err != nil {
		_ = tmp.Close()
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("save upload: %w", err))
		return
	}
	_ = tmp.Close()
	svc := importers.New(p.k, p.owner)
	asset, skipped, err := svc.ImportItem(r.Context(),
		importers.ImportItem{AbsPath: tmp.Name(), Name: hdr.Filename},
		importers.Options{Mode: importers.ModeCopy, DestSubdir: r.FormValue("dest_subdir")})
	p.writeImportResult(w, asset, skipped, err)
}

func (p *Plugin) writeImportResult(w http.ResponseWriter, asset domain.Asset, skipped bool, err error) {
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	resp := importResp{Skipped: skipped}
	if !skipped {
		dto := toDTO(asset)
		resp.Asset = &dto
	}
	httpjson.WriteJSON(w, http.StatusOK, resp)
}

// migrateReq is the JSON body for a migration import (#57).
type migrateReq struct {
	Source   string `json:"source"`   // eagle|billfish
	Path     string `json:"path"`     // library path
	Mode     string `json:"mode"`     // index|copy; empty = index
	Conflict string `json:"conflict"` // keep-both|skip
	Async    bool   `json:"async"`    // run on the JobQueue, return a job id
}

// migrate handles POST /import/migrate: opens an Eagle/Billfish library and
// imports it, synchronously (returning counts) or as a background job.
func (p *Plugin) migrate(w http.ResponseWriter, r *http.Request) {
	var req migrateReq
	if !decodeJSON(w, r, &req) {
		return
	}
	src, err := openSource(req.Source, req.Path)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	svc := importers.New(p.k, p.owner)
	opt := importers.Options{Mode: importers.Mode(req.Mode), Conflict: importers.Conflict(req.Conflict)}
	if req.Async {
		jobID, err := svc.StartMigration(r.Context(), src, opt)
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err)
			return
		}
		httpjson.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID.String()})
		return
	}
	res, err := svc.ImportSource(r.Context(), src, opt, domain.JobID{})
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, res)
}

// openSource opens a migration source library read-only.
func openSource(kind, path string) (importers.Source, error) {
	if path == "" {
		return nil, errors.New("path required")
	}
	switch kind {
	case string(importers.KindEagle):
		return eagle.Open(path)
	case string(importers.KindBillfish):
		return billfish.Open(path)
	default:
		return nil, fmt.Errorf("unknown source %q (want eagle|billfish)", kind)
	}
}

// serveRaw handles GET /assets/{id}/raw: streams the original file through the
// asset's own storage provider (Range-capable). This is provider-aware so
// migrated fs-backed assets are downloadable, not only their thumbnails.
func (p *Plugin) serveRaw(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	asset, err := p.k.Store.GetAsset(r.Context(), p.owner, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	provider, ok := p.k.Storage.Get(asset.Provider)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError,
			fmt.Errorf("provider %q not registered", asset.Provider))
		return
	}
	info, err := provider.Stat(r.Context(), asset.StoragePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := provider.Open(r.Context(), asset.StoragePath)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("open: %w", err))
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, sanitizeFilename(asset.Name)))
	http.ServeContent(w, r, asset.Name, info.ModTime, f)
}

// sanitizeFilename strips characters that could break or inject the
// Content-Disposition header (quotes, backslashes, control chars including
// CR/LF), since asset names originate from migrated/source data.
func sanitizeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
}

// jobDTO is the wire shape of a background job (import progress lives in Payload).
type jobDTO struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
}

// listJobs handles GET /jobs: lists background jobs so clients can poll a
// migration's progress (serialized in each job's payload).
func (p *Plugin) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := p.k.Store.ListJobs(r.Context(), p.owner,
		httpjson.QueryInt(r, "limit", 50), httpjson.QueryInt(r, "offset", 0))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]jobDTO, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, jobDTO{
			ID: j.ID.String(), Type: j.Type, Status: string(j.Status),
			Payload: j.Payload, CreatedAt: j.CreatedAt.Format(time.RFC3339),
		})
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}
