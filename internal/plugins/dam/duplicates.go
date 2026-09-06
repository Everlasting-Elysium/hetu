package dam

import (
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// defaultPHashThreshold is the hamming distance below which two images are
// considered visually similar. 10 bits out of 64 is a well-tested default in
// DAM systems using PerceptionHash (see goimagehash docs).
const defaultPHashThreshold = 10

// listDuplicates returns groups of live assets that share the same SHA-256
// content hash. Each group has 2+ members. Pagination applies to the group
// list, not the individual assets.
//
//	GET /api/dam/duplicates?limit=50&offset=0
func (p *Plugin) listDuplicates(w http.ResponseWriter, r *http.Request) {
	limit := httpjson.QueryInt(r, "limit", 50)
	offset := httpjson.QueryInt(r, "offset", 0)

	groups, err := p.k.Store.FindExactDuplicates(r.Context(), p.owner, limit, offset)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	type groupDTO struct {
		Hash   string     `json:"hash"`
		Assets []assetDTO `json:"assets"`
	}
	out := make([]groupDTO, 0, len(groups))
	for _, g := range groups {
		out = append(out, groupDTO{
			Hash:   g.Hash,
			Assets: toDTOs(g.Assets, nil),
		})
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// listSimilar returns groups of live image assets whose perceptual hashes are
// within a configurable hamming distance threshold. Each group has an anchor
// asset and one or more similar members with their distances.
//
//	GET /api/dam/duplicates/similar?threshold=10
func (p *Plugin) listSimilar(w http.ResponseWriter, r *http.Request) {
	threshold := httpjson.QueryInt(r, "threshold", defaultPHashThreshold)

	groups, err := p.k.Store.FindSimilarByPHash(r.Context(), p.owner, threshold)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	type hitDTO struct {
		Asset    assetDTO `json:"asset"`
		Distance int      `json:"distance"`
	}
	type groupDTO struct {
		Anchor  assetDTO `json:"anchor"`
		Members []hitDTO `json:"members"`
	}
	out := make([]groupDTO, 0, len(groups))
	for _, g := range groups {
		members := make([]hitDTO, 0, len(g.Members))
		for _, m := range g.Members {
			members = append(members, hitDTO{
				Asset:    toDTO(m.Asset),
				Distance: m.Distance,
			})
		}
		out = append(out, groupDTO{
			Anchor:  toDTO(g.Anchor),
			Members: members,
		})
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}
