package domain

// AssetFilter narrows a live-asset listing. Zero values disable a filter:
// empty FolderID/TagID match any folder/tag, MinRating 0 matches any rating,
// and an empty Kinds slice matches any format. When set, MinRating keeps assets
// rated at least that many stars and Kinds keeps assets in one of the listed
// formats. Favorite false imposes no constraint; true keeps only favorited
// assets (issue #62), the same zero-disables contract as MinRating. Status
// selects the lifecycle view: "" lists live assets (the
// default) and "missing" lists only assets whose backing file is missing from
// storage. Size (bytes), pixel dimensions (width/height), and Shapes narrow the
// same way, each zero/empty value disabling that side exactly like MinRating/
// Kinds (issue #101). Duration (seconds) and the created/indexed time ranges
// (issue #53) follow the same zero-disables-each-side contract. The same filter
// drives both ListAssetsFiltered and SearchAssets so keyword search and the
// sidebar facets compose server-side.
type AssetFilter struct {
	FolderID  string
	TagID     string
	MinRating int
	Favorite  bool        // false = no favorite constraint; true = keep only favorited assets
	Kinds     []AssetKind // empty = any format; else keep assets whose kind is in the set
	Status    string      // "" = normal (live), "missing" = missing files only

	// Size (bytes, anchor row — see appendFacetConds) and pixel dimensions /
	// shape (current-version resolved) narrowing, issue #101. Zero/empty
	// disables each: MinSize/MaxSize/MinWidth/MaxWidth/MinHeight/MaxHeight 0
	// means unbounded on that side; empty Shapes matches any shape.
	MinSize, MaxSize                         int64
	MinWidth, MaxWidth, MinHeight, MaxHeight int
	Shapes                                   []AssetShape

	// Duration (seconds) and created/indexed time-range narrowing, issue #53.
	// MinDuration/MaxDuration range over the audio.duration/video.duration
	// annotation (float64 seconds — see appendFacetConds' durationJoin); 0
	// disables that side, so images/documents (no duration) never match a
	// duration filter. CreatedAfter/CreatedBefore and IndexedAfter/IndexedBefore
	// are unix-second bounds on assets.created_at / assets.indexed_at (the
	// INTEGER columns); 0 disables that side. Each side follows the same
	// contract as the size/dimension ranges (negative -> unbounded, inverted
	// max<min -> drop max; normalized in parseAssetFilter).
	MinDuration, MaxDuration    float64
	CreatedAfter, CreatedBefore int64
	IndexedAfter, IndexedBefore int64

	// Sort selects the ORDER BY (issue #114); "" = indexed_at DESC, the
	// unchanged default every DAM caller relies on. See domain.AssetSort.
	Sort AssetSort

	// CollectionID narrows to a single collection's members (issue #114's
	// optional ?collection= on /list,/random,/daily). Empty = no collection
	// constraint. The /collections/{id} endpoint does NOT use this field — it
	// calls the dedicated ListCollectionAssets store method instead, ordered by
	// the collection's manual ord rather than Sort.
	CollectionID string
}
