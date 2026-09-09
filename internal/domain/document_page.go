package domain

// DocumentPage is one rendered page of a multi-page document (PDF, or an office
// file converted to PDF). Each page carries its own thumbnail so the DAM UI can
// flip through a document without re-rendering (issue #48). Pages are rebuilt
// wholesale on every scan (see Store.ReplaceDocumentPages), so a page count that
// shrinks never leaves stale rows behind.
type DocumentPage struct {
	AssetID   AssetID
	PageNo    int // 1-based
	ThumbPath string
	Width     int
	Height    int
}
