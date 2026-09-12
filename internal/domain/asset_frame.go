package domain

// AssetFrame is one frame of an image sequence (issue #62): a run of
// consecutively-numbered image files (explosion_0001.png, explosion_0002.png,
// ...) is indexed as a single sequence asset anchored at the lowest-numbered
// frame, rather than one asset per file. Every frame — including the anchor
// (FrameNo 1) — gets a row so the detail-view stepper reads them all from one
// place. No per-frame thumbnail is generated; a step serves the original frame
// bytes on demand (StoragePath). Frames are rebuilt wholesale on every scan (see
// Store.ReplaceAssetFrames), so a sequence that gains or loses frames never
// leaves stale rows behind.
type AssetFrame struct {
	AssetID     AssetID
	FrameNo     int // 1-based, ascending by the frame's parsed number
	StoragePath string
	Name        string
}
