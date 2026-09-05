package domain

// AssetFilter narrows a live-asset listing. Zero values disable a filter:
// empty FolderID/TagID match any folder/tag, and MinRating 0 matches any
// rating. When set, MinRating keeps assets rated at least that many stars.
type AssetFilter struct {
	FolderID  string
	TagID     string
	MinRating int
}
