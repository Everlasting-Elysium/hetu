package dam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// renameMode discriminates the batch-rename strategies.
type renameMode string

const (
	renameSimple      renameMode = ""             // set display_name on all
	renameSequence    renameMode = "sequence"     // template with {n}/{date}
	renameFindReplace renameMode = "find_replace" // substring replace on current name
)

type renameRequest struct {
	AssetIDs    []string `json:"asset_ids"`
	Pattern     string   `json:"pattern"`
	DisplayName string   `json:"display_name"` // simple mode
	Template    string   `json:"template"`     // sequence mode
	Start       int      `json:"start"`        // sequence mode
	Find        string   `json:"find"`         // find_replace mode
	Replace     string   `json:"replace"`      // find_replace mode
}

func (p *Plugin) batchRename(w http.ResponseWriter, r *http.Request) {
	var req renameRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ids, err := parseAssetIDs(req.AssetIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	status, err := p.doRename(r.Context(), ids, req)
	if err != nil {
		httpjson.WriteError(w, status, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"renamed": len(ids)})
}

// doRename dispatches on the rename mode and returns the HTTP status to use if
// it fails. On success it returns (http.StatusOK, nil).
func (p *Plugin) doRename(ctx context.Context, ids []domain.AssetID, req renameRequest) (int, error) {
	switch renameMode(req.Pattern) {
	case renameSimple:
		if err := p.k.Store.BatchUpdateDisplayName(ctx, p.owner, ids, req.DisplayName); err != nil {
			return http.StatusInternalServerError, err
		}
		return http.StatusOK, nil
	case renameSequence:
		if req.Template == "" {
			return http.StatusBadRequest, errors.New("template required for sequence rename")
		}
		return p.applyRenames(ctx, ids, func(assets []domain.Asset) map[domain.AssetID]string {
			return sequenceRenames(assets, req.Template, req.Start, time.Now().UTC())
		})
	case renameFindReplace:
		if req.Find == "" {
			return http.StatusBadRequest, errors.New("find required for find_replace rename")
		}
		return p.applyRenames(ctx, ids, func(assets []domain.Asset) map[domain.AssetID]string {
			return findReplaceRenames(assets, req.Find, req.Replace)
		})
	default:
		return http.StatusBadRequest, fmt.Errorf("unknown rename pattern %q", req.Pattern)
	}
}

// applyRenames fetches the live assets in ids, computes per-asset display names,
// and persists them in one transaction.
func (p *Plugin) applyRenames(ctx context.Context, ids []domain.AssetID, compute func([]domain.Asset) map[domain.AssetID]string) (int, error) {
	assets, err := p.fetchAssets(ctx, ids)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if err := p.k.Store.BatchRenameDisplayNames(ctx, p.owner, compute(assets)); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

// fetchAssets loads each id in order, skipping ids that do not resolve to a
// live asset owned by the plugin owner.
func (p *Plugin) fetchAssets(ctx context.Context, ids []domain.AssetID) ([]domain.Asset, error) {
	assets := make([]domain.Asset, 0, len(ids))
	for _, id := range ids {
		a, err := p.k.Store.GetAsset(ctx, p.owner, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, nil
}

// sequenceRenames renders template for each asset in order, substituting {n}
// (start, start+1, ...) and {date} (now as YYYY-MM-DD).
func sequenceRenames(assets []domain.Asset, template string, start int, now time.Time) map[domain.AssetID]string {
	date := now.Format("2006-01-02")
	out := make(map[domain.AssetID]string, len(assets))
	for i, a := range assets {
		name := strings.ReplaceAll(template, "{n}", strconv.Itoa(start+i))
		name = strings.ReplaceAll(name, "{date}", date)
		out[a.ID] = name
	}
	return out
}

// findReplaceRenames replaces every occurrence of find with replace in each
// asset's current effective name (display_name, falling back to name).
func findReplaceRenames(assets []domain.Asset, find, replace string) map[domain.AssetID]string {
	out := make(map[domain.AssetID]string, len(assets))
	for _, a := range assets {
		base := a.DisplayName
		if base == "" {
			base = a.Name
		}
		out[a.ID] = strings.ReplaceAll(base, find, replace)
	}
	return out
}
