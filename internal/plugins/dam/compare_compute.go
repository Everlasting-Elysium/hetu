package dam

import (
	"context"
	"log/slog"
	"math"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/imgcompare"
)

// dimensionSet is the set of comparison dimensions selected for one request.
type dimensionSet map[imgcompare.Dimension]bool

func (s dimensionSet) has(d imgcompare.Dimension) bool { return s[d] }

// allDimensions is the canonical order: it defines the default selection and the
// stable Dimensions list order regardless of map iteration order.
var allDimensions = []imgcompare.Dimension{
	imgcompare.DimColor, imgcompare.DimTone, imgcompare.DimLighting,
	imgcompare.DimAction, imgcompare.DimElement,
}

// parseDimensions reads the optional comma-separated dimensions value. Unknown
// tokens are ignored; an empty or all-unknown value selects all five dimensions.
func parseDimensions(raw string) dimensionSet {
	if strings.TrimSpace(raw) == "" {
		return allDimensionSet()
	}
	set := dimensionSet{}
	for _, tok := range strings.Split(raw, ",") {
		d := imgcompare.Dimension(strings.ToLower(strings.TrimSpace(tok)))
		if knownDimension(d) {
			set[d] = true
		}
	}
	if len(set) == 0 {
		return allDimensionSet()
	}
	return set
}

func allDimensionSet() dimensionSet {
	set := make(dimensionSet, len(allDimensions))
	for _, d := range allDimensions {
		set[d] = true
	}
	return set
}

func knownDimension(d imgcompare.Dimension) bool {
	for _, k := range allDimensions {
		if k == d {
			return true
		}
	}
	return false
}

// dimensionNames lists the dimensions actually scored, in canonical order, so
// the response reports what was computed (not merely what was requested).
func dimensionNames(scores map[imgcompare.Dimension]float64) []string {
	out := make([]string, 0, len(scores))
	for _, d := range allDimensions {
		if _, ok := scores[d]; ok {
			out = append(out, string(d))
		}
	}
	return out
}

// compareRequest is a fully-resolved comparison ready to score: the request id,
// the selected dimensions, and both resolved sides.
type compareRequest struct {
	reqID    string
	selected dimensionSet
	ref, tgt sideData
}

// compareRun accumulates one comparison's results. Bundling the response and the
// score map on a receiver keeps the per-dimension steps parameter-free and lets
// each step set only the fields it owns.
type compareRun struct {
	p      *Plugin
	req    compareRequest
	resp   compareResponse
	scores map[imgcompare.Dimension]float64
}

// computeCompare runs the selected dimensions and assembles the response. Only
// dimensions that were both requested and successfully computed contribute to
// Dimensions and the aggregate OverallScore.
func (p *Plugin) computeCompare(ctx context.Context, req compareRequest) compareResponse {
	run := &compareRun{
		p:   p,
		req: req,
		resp: compareResponse{
			ReqID: req.reqID,
			Overlays: compareOverlays{
				ReferenceURL: p.overlayURL(req.reqID, sideReference),
				TargetURL:    p.overlayURL(req.reqID, sideTarget),
			},
			Critique: compareCritique{Available: false},
		},
		scores: make(map[imgcompare.Dimension]float64, len(allDimensions)),
	}
	run.color()
	run.luma()
	run.semantic(ctx)
	run.resp.Dimensions = dimensionNames(run.scores)
	run.resp.OverallScore = round2(imgcompare.Aggregate(run.scores))
	run.critique(ctx)
	return run.resp
}

// color runs the palette dimension when selected.
func (run *compareRun) color() {
	if !run.req.selected.has(imgcompare.DimColor) {
		return
	}
	cr := imgcompare.Color(run.req.ref.img, run.req.tgt.img)
	run.resp.Color = &cr
	run.scores[imgcompare.DimColor] = cr.Score
}

// luma runs the two luma-driven dimensions. When both tone and lighting are
// selected it uses the shared single-pass entry point (ToneAndLighting) so the
// per-image luma field is built once.
func (run *compareRun) luma() {
	wantTone := run.req.selected.has(imgcompare.DimTone)
	wantLight := run.req.selected.has(imgcompare.DimLighting)
	ref, tgt := run.req.ref.img, run.req.tgt.img
	switch {
	case wantTone && wantLight:
		tr, lr := imgcompare.ToneAndLighting(ref, tgt)
		run.resp.Tone, run.resp.Lighting = &tr, &lr
		run.scores[imgcompare.DimTone] = tr.Score
		run.scores[imgcompare.DimLighting] = lr.Score
	case wantTone:
		tr := imgcompare.Tone(ref, tgt)
		run.resp.Tone = &tr
		run.scores[imgcompare.DimTone] = tr.Score
	case wantLight:
		lr := imgcompare.Lighting(ref, tgt)
		run.resp.Lighting = &lr
		run.scores[imgcompare.DimLighting] = lr.Score
	}
}

// semantic runs the tag-driven action/element dimensions. Both need tags from
// both sides; if either side's tags are unavailable (no Tagger, or a Tagger call
// errored) these dimensions are omitted entirely rather than reported as a
// misleading empty-set perfect score — a single failed external call must never
// fail the whole request.
func (run *compareRun) semantic(ctx context.Context) {
	sel := run.req.selected
	if !sel.has(imgcompare.DimAction) && !sel.has(imgcompare.DimElement) {
		return
	}
	refSem := run.p.sideSemantics(ctx, run.req.ref)
	tgtSem := run.p.sideSemantics(ctx, run.req.tgt)
	if !refSem.tagsOK || !tgtSem.tagsOK {
		return
	}
	if sel.has(imgcompare.DimAction) {
		ar := imgcompare.Action(refSem.tags, tgtSem.tags)
		run.resp.Action = &ar
		run.scores[imgcompare.DimAction] = ar.Score
	}
	if sel.has(imgcompare.DimElement) {
		er := imgcompare.Element(
			imgcompare.ElementInput{Tags: refSem.tags, Embedding: refSem.embedding},
			imgcompare.ElementInput{Tags: tgtSem.tags, Embedding: tgtSem.embedding},
		)
		run.resp.Element = &er
		run.scores[imgcompare.DimElement] = er.Score
	}
}

// semanticFeatures are one side's tag map and embedding for the action/element
// dimensions. tagsOK is false when no Tagger is configured or the call failed,
// which flags the dimension unavailable rather than silently scoring empty tags.
type semanticFeatures struct {
	tags      map[string]float64
	tagsOK    bool
	embedding []float32
}

// sideSemantics fetches a side's tags (via the optional Tagger) and embedding
// (reusing a library asset's stored vector, else the optional Embedder). Every
// external failure is logged and downgraded to "unavailable"; nothing here can
// fail the request.
func (p *Plugin) sideSemantics(ctx context.Context, side sideData) semanticFeatures {
	sf := semanticFeatures{embedding: side.embedding}
	if p.k.Tagger != nil {
		tags, err := p.k.Tagger.Tag(ctx, side.assetRef)
		if err != nil {
			p.k.Log.WarnContext(ctx, "compare: tag unavailable",
				slog.String("ref", side.assetRef), slog.Any("err", err))
		} else {
			sf.tags, sf.tagsOK = tags, true
		}
	}
	if sf.embedding == nil && p.k.Embedder != nil {
		emb, err := p.k.Embedder.Embed(ctx, side.assetRef)
		if err != nil {
			p.k.Log.WarnContext(ctx, "compare: embed unavailable",
				slog.String("ref", side.assetRef), slog.Any("err", err))
		} else {
			sf.embedding = emb
		}
	}
	return sf
}

// round2 rounds a 0-100 score to two decimals for a stable JSON payload.
func round2(x float64) float64 { return math.Round(x*100) / 100 }
