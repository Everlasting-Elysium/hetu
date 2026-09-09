package domain

// AssetFilter narrows a live-asset listing. Zero values disable a filter:
// empty FolderID/TagID match any folder/tag, MinRating 0 matches any rating,
// and an empty Kinds slice matches any format. When set, MinRating keeps assets
// rated at least that many stars and Kinds keeps assets in one of the listed
// formats. Status selects the lifecycle view: "" lists live assets (the
// default) and "missing" lists only assets whose backing file is missing from
// storage. Size (bytes), pixel dimensions (width/height), and Shapes narrow the
// same way, each zero/empty value disabling that side exactly like MinRating/
// Kinds (issue #101). The same filter drives both ListAssetsFiltered and
// SearchAssets so keyword search and the sidebar facets compose server-side.
type AssetFilter struct {
	FolderID  string
	TagID     string
	MinRating int
	Kinds     []AssetKind // empty = any format; else keep assets whose kind is in the set
	Status    string      // "" = normal (live), "missing" = missing files only

	// Size (bytes, anchor row — see appendFacetConds) and pixel dimensions /
	// shape (current-version resolved) narrowing, issue #101. Zero/empty
	// disables each: MinSize/MaxSize/MinWidth/MaxWidth/MinHeight/MaxHeight 0
	// means unbounded on that side; empty Shapes matches any shape.
	MinSize, MaxSize                         int64
	MinWidth, MaxWidth, MinHeight, MaxHeight int
	Shapes                                   []AssetShape
}
