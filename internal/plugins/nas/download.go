package nas

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// download handles GET /api/nas/download?path=. It streams the file via
// http.ServeContent which handles Range requests, Content-Length, and
// conditional GETs. The path is cleaned by the storage provider so callers
// cannot escape the root.
func (p *Plugin) download(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("missing path parameter"))
		return
	}
	p.serveFile(w, r, path)
}

// serveFile streams a file from the storage provider with Range support.
// Shared by the download endpoint and the public share access handler.
func (p *Plugin) serveFile(w http.ResponseWriter, r *http.Request, path string) {
	provider, ok := p.k.Storage.Get(p.provider)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError,
			fmt.Errorf("storage provider %q not registered", p.provider))
		return
	}

	info, err := provider.Stat(r.Context(), path)
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, fmt.Errorf("file not found: %w", err))
		return
	}
	if info.IsDir {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("path is a directory"))
		return
	}

	f, err := provider.Open(r.Context(), path)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("open file: %w", err))
		return
	}
	defer f.Close()

	name := filepath.Base(path)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	http.ServeContent(w, r, name, info.ModTime, f)
}

// serveDir lists entries of a directory from the storage provider. Used by
// the public share access handler for shared folders.
func (p *Plugin) serveDir(w http.ResponseWriter, r *http.Request, path string) {
	provider, ok := p.k.Storage.Get(p.provider)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError,
			fmt.Errorf("storage provider %q not registered", p.provider))
		return
	}
	entries, err := provider.List(r.Context(), path)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTOs(entries))
}

