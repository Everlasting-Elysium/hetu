package domain

import "time"

// AssetKind is the coarse category of an asset, stored as a string in the DB.
type AssetKind string

const (
	KindImage    AssetKind = "image"
	KindVideo    AssetKind = "video"
	KindAudio    AssetKind = "audio"
	KindModel    AssetKind = "model" // 3D models (obj/fbx/glb/stl/ztl/zpr...)
	KindDocument AssetKind = "document"
	KindOther    AssetKind = "other"
)

// Asset is an indexed resource. Files are indexed in place (referenced by
// StoragePath), never copied into hetu's own storage.
type Asset struct {
	ID          AssetID
	Owner       OwnerID
	Kind        AssetKind
	Provider    string // storage provider name, e.g. "local"
	StoragePath string
	Name        string
	Ext         string
	Size        int64
	Hash        string // content hash for dedup
	ThumbPath   string
	Width       int
	Height      int
	CreatedAt   time.Time
	IndexedAt   time.Time
}
