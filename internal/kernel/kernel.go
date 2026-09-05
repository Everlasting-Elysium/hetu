package kernel

import "log/slog"

// Kernel holds the shared services every plugin consumes.
type Kernel struct {
	Log         *slog.Logger
	Store       Store
	Storage     *StorageRegistry
	Assets      *AssetRegistry
	Events      *EventBus
	Jobs        *JobQueue
	ThumbDir    string // directory where generated thumbnails are written
	BlenderAddr string // host:port of the Blender sidecar; empty = disabled
}

// Deps are the externally provided dependencies for New.
type Deps struct {
	Log         *slog.Logger
	Store       Store
	ThumbDir    string
	JobBuffer   int
	BlenderAddr string
}

// New constructs a Kernel with empty registries and an idle job queue.
func New(d Deps) *Kernel {
	return &Kernel{
		Log:         d.Log,
		Store:       d.Store,
		Storage:     NewStorageRegistry(),
		Assets:      NewAssetRegistry(),
		Events:      NewEventBus(),
		Jobs:        NewJobQueue(d.Log, d.JobBuffer),
		ThumbDir:    d.ThumbDir,
		BlenderAddr: d.BlenderAddr,
	}
}
