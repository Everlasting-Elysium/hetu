package domain

// AssetFilter narrows a live-asset listing. Zero values disable a filter:
// empty FolderID/TagID match any folder/tag, and MinRating 0 matches any
// rating. When set, MinRating keeps assets rated at least that many stars.
// Status selects the lifecycle view: "" lists live assets (the default) and
// "missing" lists only assets whose backing file is missing from storage.
type AssetFilter struct {
	FolderID  string
	TagID     string
	MinRating int
	Status    string // "" = normal (live), "missing" = missing files only
}
