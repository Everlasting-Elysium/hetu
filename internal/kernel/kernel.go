package kernel

import (
	"context"
	"log/slog"
)

// Embedder produces CLIP embeddings for text or image references. It is
// optionally set on the kernel when an AI sidecar is configured; nil means
// semantic search is unavailable.
type Embedder interface {
	Embed(ctx context.Context, ref string) ([]float32, error)
}

// Kernel holds the shared services every plugin consumes.
type Kernel struct {
	Log         *slog.Logger
	Store       Store
	Storage     *StorageRegistry
	Assets      *AssetRegistry
	Events      *EventBus
	Jobs        *JobQueue
	ThumbDir    string   // directory where generated thumbnails are written
	BlenderAddr string   // host:port of the Blender sidecar; empty = disabled
	Embedder    Embedder // optional CLIP embedder; nil = semantic search disabled
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
