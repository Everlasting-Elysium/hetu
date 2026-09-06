package dam

import (
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

var (
	errNoEmbedder    = errors.New("semantic search unavailable: AI sidecar not configured (set HETU_AI_ADDR)")
	errEmptySemantic = errors.New("semantic: query must not be empty")
)

// searchBySemantic handles GET /api/dam/search?semantic=<text>: embed the query
// text via the AI sidecar, then brute-force cosine search over stored CLIP
// vectors.
func (p *Plugin) searchBySemantic(w http.ResponseWriter, r *http.Request) {
	if p.k.Embedder == nil {
		httpjson.WriteError(w, http.StatusServiceUnavailable, errNoEmbedder)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("semantic"))
	if query == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errEmptySemantic)
		return
	}

	limit := httpjson.QueryInt(r, "limit", searchDefaultLimit)
	if limit <= 0 || limit > searchMaxLimit {
		limit = searchDefaultLimit
	}

	vector, err := p.k.Embedder.Embed(r.Context(), query)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, err)
		return
	}

	matches, err := p.k.Store.SearchByEmbedding(r.Context(), p.owner, vector, domain.AssetID{}, limit)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	ids := make([]domain.AssetID, len(matches))
	for i, m := range matches {
		ids[i] = m.Asset.ID
	}
	notes, err := p.fetchNotes(r.Context(), ids)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toSimilarityDTOs(matches, notes))
}

// searchBySimilar handles GET /api/dam/search?similar=<asset_id>: look up the
// stored embedding for the given asset and find visually similar assets.
func (p *Plugin) searchBySimilar(w http.ResponseWriter, r *http.Request) {
	rawID := r.URL.Query().Get("similar")
	assetID, err := domain.NewAssetID(rawID)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}

	limit := httpjson.QueryInt(r, "limit", searchDefaultLimit)
	if limit <= 0 || limit > searchMaxLimit {
		limit = searchDefaultLimit
	}

	vector, err := p.k.Store.GetEmbedding(r.Context(), assetID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpjson.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	// Exclude the query asset itself: it scores 1.0 against its own vector.
	matches, err := p.k.Store.SearchByEmbedding(r.Context(), p.owner, vector, assetID, limit)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	simIDs := make([]domain.AssetID, len(matches))
	for i, m := range matches {
		simIDs[i] = m.Asset.ID
	}
	notes, err := p.fetchNotes(r.Context(), simIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toSimilarityDTOs(matches, notes))
}

// similarityDTO is an asset with its cosine similarity score.
type similarityDTO struct {
	assetDTO
	Similarity float64 `json:"similarity"`
}

func toSimilarityDTOs(matches []domain.SimilarityMatch, notes map[string]string) []similarityDTO {
	out := make([]similarityDTO, 0, len(matches))
	for _, m := range matches {
		dto := similarityDTO{
			assetDTO:   toDTO(m.Asset),
			Similarity: math.Round(m.Similarity*10000) / 10000,
		}
		if notes != nil {
			dto.Note = notes[m.Asset.ID.String()]
		}
		out = append(out, dto)
	}
	return out
}
