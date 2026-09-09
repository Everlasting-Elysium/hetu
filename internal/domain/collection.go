package domain

import "fmt"

// CollectionID identifies a collection (a manual grouping).
type CollectionID struct{ raw string }

// NewCollectionID parses s into a CollectionID.
func NewCollectionID(s string) (CollectionID, error) {
	if s == "" {
		return CollectionID{}, fmt.Errorf("collection id: %w", ErrEmptyID)
	}
	return CollectionID{raw: s}, nil
}

// String returns the raw collection id.
func (id CollectionID) String() string { return id.raw }

// Collection is a manual, user-curated grouping of assets, independent of the
// folder tree. It differs from the two existing organizers: a Folder is an
// asset's single physical home (one parent, a real path), while a Tag is a flat
// many-to-many label. A Collection nests via ParentID and its members carry an
// explicit manual order (see CollectionItem.Ord); an asset may sit in many
// collections. Cover is an optional asset_id override; when empty the store
// derives the effective cover from the lowest-ord member (ListCollections
// resolves it, GetCollection leaves it raw so the edit path stays lossless).
type Collection struct {
	ID       CollectionID
	Owner    OwnerID
	ParentID string
	Name     string
	Cover    string
}

// CollectionItem is a collection membership enriched with its asset's display
// fields. It is a store→handler transport value carried in the domain layer,
// unlike board items which enrich in the handler via per-item GetAsset. That
// difference is deliberate: a board item may reference a since-deleted asset and
// mixes note/asset kinds, so it tolerates a missing lookup per item; every
// collection_item references a live asset row, so the store resolves
// kind/name/thumb for the whole list in one JOIN query (avoiding N+1) and hands
// back this typed value.
type CollectionItem struct {
	AssetID    AssetID
	Ord        int
	AssetKind  string
	AssetName  string
	AssetThumb string
}
