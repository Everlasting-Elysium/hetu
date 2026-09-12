package dam

import (
	"context"
	"log/slog"
)

// critique asks the optional VLM VisionCritic to compare the two images across
// the dimensions actually computed for this request (run.resp.Dimensions, not
// merely what was requested). It is best-effort: a nil VisionCritic (none
// configured), a 501 (sidecar has no VLM loaded, surfaced as
// ai.ErrNotImplemented), a timeout, or any other error is logged and downgraded
// to an unavailable critique — a single failed external call must never fail the
// whole /compare request. On success resp.Critique carries the summary,
// per-dimension notes, and producing model.
func (run *compareRun) critique(ctx context.Context) {
	if run.p.k.VisionCritic == nil {
		return
	}
	cr, err := run.p.k.VisionCritic.Critique(ctx, run.req.ref.assetRef, run.req.tgt.assetRef, run.resp.Dimensions)
	if err != nil {
		run.p.k.Log.WarnContext(ctx, "compare: critique unavailable",
			slog.String("ref", run.req.ref.assetRef),
			slog.String("target", run.req.tgt.assetRef),
			slog.Any("err", err))
		return
	}
	run.resp.Critique = compareCritique{
		Available:  true,
		Summary:    cr.Summary,
		Dimensions: cr.Dimensions,
		Model:      cr.Model,
	}
}
