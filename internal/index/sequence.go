package index

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// minSequenceFrames is the smallest run of consecutively-numbered image files
// that counts as a sequence; a lone numbered file is an ordinary asset.
const minSequenceFrames = 2

// numberedFile is an image entry whose name ends in digits, split into the stem
// (everything before the trailing digits) and the parsed number.
type numberedFile struct {
	entry domain.Entry
	stem  string
	ext   string
	num   int
}

// indexFiles groups one directory's files into image sequences and standalone
// files (issue #62), then indexes each: a sequence becomes ONE asset (its
// anchor) plus a frame index, every other file is indexed on its own. It returns
// early only when the scan context is cancelled, so a single unsupported or
// broken file never aborts the walk (indexEntry/indexSequence log and skip).
func (ix *Indexer) indexFiles(ctx context.Context, p kernel.StorageProvider, files []domain.Entry, res *ScanResult) error {
	groups, singles := groupSequences(files, ix.isImageExt)
	for _, e := range singles {
		if err := ctx.Err(); err != nil {
			return err
		}
		ix.indexEntry(ctx, p, e, res)
	}
	for _, g := range groups {
		if err := ctx.Err(); err != nil {
			return err
		}
		ix.indexSequence(ctx, p, g, res)
	}
	return nil
}

// indexEntry indexes one standalone file and updates the scan tally, mirroring
// the per-file body the walk used before sequence grouping. A successfully
// indexed image also clears any frame rows a previous scan wrote when this file
// was still part of a sequence that has since shrunk to one file, so a former
// anchor never keeps stale frames.
func (ix *Indexer) indexEntry(ctx context.Context, p kernel.StorageProvider, e domain.Entry, res *ScanResult) {
	reconnected, err := ix.indexOne(ctx, p, e)
	if err != nil {
		ix.k.Log.WarnContext(ctx, "index skip",
			slog.String("path", e.Path), slog.Any("err", err))
		res.Skipped++
		return
	}
	if ix.isImageExt(extOf(e.Name)) {
		ix.clearFrames(ctx, p, e.Path)
	}
	if reconnected {
		res.Reconnected++
		return
	}
	res.Indexed++
}

// indexSequence indexes an image sequence as ONE asset: it indexes the anchor
// (the lowest-numbered frame, group[0]) through the normal single-file chain —
// asset row, thumbnail, palette, pHash, metadata — then records every frame
// (including the anchor, frame 1) in the frame index. The absorbed frames 2..N
// never become their own assets. A sequence counts as one Indexed (or
// Reconnected) asset; a failed anchor index skips the whole group.
func (ix *Indexer) indexSequence(ctx context.Context, p kernel.StorageProvider, group []domain.Entry, res *ScanResult) {
	anchor := group[0]
	reconnected, err := ix.indexOne(ctx, p, anchor)
	if err != nil {
		ix.k.Log.WarnContext(ctx, "index skip",
			slog.String("path", anchor.Path), slog.Any("err", err))
		res.Skipped++
		return
	}
	ix.indexFrames(ctx, p, anchor.Path, group)
	if reconnected {
		res.Reconnected++
		return
	}
	res.Indexed++
	ix.k.Log.InfoContext(ctx, "indexed sequence",
		slog.String("anchor", anchor.Path), slog.Int("frames", len(group)))
}

// indexFrames records group as the anchor asset's frame index (frame 1 is the
// anchor). Frames are addressed by the anchor's natural key and rebuilt
// wholesale (see Store.ReplaceAssetFrames); a store failure is logged, never
// fatal to the scan.
func (ix *Indexer) indexFrames(ctx context.Context, p kernel.StorageProvider, anchorPath string, group []domain.Entry) {
	frames := make([]domain.AssetFrame, 0, len(group))
	for i, e := range group {
		frames = append(frames, domain.AssetFrame{
			FrameNo:     i + 1,
			StoragePath: e.Path,
			Name:        e.Name,
		})
	}
	if err := ix.k.Store.ReplaceAssetFrames(ctx, ix.owner, p.Name(), anchorPath, frames); err != nil {
		ix.k.Log.WarnContext(ctx, "frames store",
			slog.String("path", anchorPath), slog.Any("err", err))
	}
}

// clearFrames drops any frame rows for the file at path (it is no longer part of
// a sequence). Best-effort, mirroring indexPages' clearPages.
func (ix *Indexer) clearFrames(ctx context.Context, p kernel.StorageProvider, path string) {
	if err := ix.k.Store.ReplaceAssetFrames(ctx, ix.owner, p.Name(), path, nil); err != nil {
		ix.k.Log.WarnContext(ctx, "frames clear",
			slog.String("path", path), slog.Any("err", err))
	}
}

// isImageExt reports whether ext resolves to an image handler, so only runs of
// images are ever collapsed into a sequence (a numbered run of PDFs or videos
// stays individual assets).
func (ix *Indexer) isImageExt(ext string) bool {
	h, ok := ix.k.Assets.HandlerFor(ext)
	return ok && h.Kind() == domain.KindImage
}

// groupSequences partitions a directory's files into image sequence groups and
// standalone singles (issue #62). A sequence is minSequenceFrames+ image files
// that share a stem and an extension and whose trailing numbers form a gap-free
// run (1,2,3 or 0001,0002,0003 — zero-padding is tolerated because the number is
// parsed, not string-compared, so directory order is irrelevant). Each returned
// group is ordered by number ascending, so group[0] is the anchor. Every file
// that is not part of such a run — a non-image, a name without trailing digits,
// an isolated number, or one side of a gap — is returned as a single for
// ordinary per-file indexing.
//
// The rule is purely name-based, so it cannot tell an animation's frames from
// consecutively-numbered camera photos (IMG_1234.jpg, IMG_1235.jpg, ...) — such
// a burst is collapsed into one sequence asset. Accepted for the "basically
// works" scope (issue #62); a later config switch or manual split can refine it.
func groupSequences(files []domain.Entry, isImage func(ext string) bool) (groups [][]domain.Entry, singles []domain.Entry) {
	buckets := make(map[string][]numberedFile)
	var order []string // first-seen bucket order keeps output deterministic
	for _, e := range files {
		nf, ok := parseNumbered(e, isImage)
		if !ok {
			singles = append(singles, e)
			continue
		}
		key := nf.stem + "\x00" + nf.ext
		if _, seen := buckets[key]; !seen {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], nf)
	}
	for _, key := range order {
		bucket := buckets[key]
		sort.SliceStable(bucket, func(i, j int) bool { return bucket[i].num < bucket[j].num })
		for _, run := range consecutiveRuns(bucket) {
			if len(run) >= minSequenceFrames {
				groups = append(groups, entriesOf(run))
				continue
			}
			for _, nf := range run {
				singles = append(singles, nf.entry)
			}
		}
	}
	return groups, singles
}

// parseNumbered splits an image entry into its stem and trailing number. ok is
// false for a non-image, or a name with no trailing digits (or digits that
// overflow an int) — such a file is never a sequence candidate.
func parseNumbered(e domain.Entry, isImage func(ext string) bool) (numberedFile, bool) {
	ext := extOf(e.Name)
	if ext == "" || !isImage(ext) {
		return numberedFile{}, false
	}
	base := strings.TrimSuffix(e.Name, filepath.Ext(e.Name))
	i := len(base)
	for i > 0 && base[i-1] >= '0' && base[i-1] <= '9' {
		i--
	}
	digits := base[i:]
	if digits == "" {
		return numberedFile{}, false
	}
	num, err := strconv.Atoi(digits)
	if err != nil {
		return numberedFile{}, false
	}
	return numberedFile{entry: e, stem: base[:i], ext: ext, num: num}, true
}

// consecutiveRuns splits a number-sorted bucket into maximal runs where each
// number is exactly one greater than the previous. A gap starts a new run, and a
// repeated number (e.g. zero-padded "01" beside "1") also breaks it, so an
// ambiguous duplicate is never folded into a sequence.
func consecutiveRuns(sorted []numberedFile) [][]numberedFile {
	var runs [][]numberedFile
	var cur []numberedFile
	for i, nf := range sorted {
		if i > 0 && nf.num == sorted[i-1].num+1 {
			cur = append(cur, nf)
			continue
		}
		if len(cur) > 0 {
			runs = append(runs, cur)
		}
		cur = []numberedFile{nf}
	}
	if len(cur) > 0 {
		runs = append(runs, cur)
	}
	return runs
}

// entriesOf extracts the entries from a run, preserving its number order.
func entriesOf(run []numberedFile) []domain.Entry {
	out := make([]domain.Entry, len(run))
	for i, nf := range run {
		out[i] = nf.entry
	}
	return out
}

// extOf returns name's lowercased extension without the leading dot.
func extOf(name string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
}
