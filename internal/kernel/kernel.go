package kernel

import (
	"context"
	"io"
	"log/slog"
)

// Embedder produces CLIP embeddings for text or image references. It is
// optionally set on the kernel when an AI sidecar is configured; nil means
// semantic search is unavailable.
type Embedder interface {
	Embed(ctx context.Context, ref string) ([]float32, error)
}

// Tagger produces auto-tags (name -> confidence) for an image reference. It
// is optionally set on the kernel when an AI sidecar is configured; nil
// means synchronous tagging is unavailable (the async scan-time tagging
// pipeline via ai.Subscribe is unaffected either way).
type Tagger interface {
	Tag(ctx context.Context, ref string) (map[string]float64, error)
}

// ModelConverter converts a 3D model (identified by ext, lowercase no dot) to
// GLB, streaming the result into w. It is defined here — not in the model3d
// package — so the kernel can hold the abstraction without importing model3d
// (which imports the kernel). A nil ModelConverter means conversion is
// unavailable and callers must degrade gracefully.
type ModelConverter interface {
	ConvertToGLB(ctx context.Context, ext string, src io.ReadSeeker, w io.Writer) error
}

// Kernel holds the shared services every plugin consumes.
type Kernel struct {
	Log           *slog.Logger
	Store         Store
	Storage       *StorageRegistry
	Assets        *AssetRegistry
	Events        *EventBus
	Jobs          *JobQueue
	ThumbDir      string   // directory where generated thumbnails are written
	CompareDir    string   // directory where transient compare uploads are staged (issue #127)
	ModelCacheDir string   // directory where web-friendly GLB conversions are cached
	Embedder      Embedder // optional CLIP embedder; nil = semantic search disabled
	Tagger        Tagger   // optional WD tagger; nil = synchronous tagging disabled
	// ModelConverter converts non-web-friendly 3D models to GLB for the viewer.
	// nil = conversion unavailable; the DAM plugin gates on it (see serveModel).
	ModelConverter ModelConverter
}

// Deps are the externally provided dependencies for New.
type Deps struct {
	Log            *slog.Logger
	Store          Store
	ThumbDir       string
	CompareDir     string
	ModelCacheDir  string
	JobBuffer      int
	ModelConverter ModelConverter
}

// New constructs a Kernel with empty registries and an idle job queue.
func New(d Deps) *Kernel {
	return &Kernel{
		Log:            d.Log,
		Store:          d.Store,
		Storage:        NewStorageRegistry(),
		Assets:         NewAssetRegistry(),
		Events:         NewEventBus(),
		Jobs:           NewJobQueue(d.Log, d.JobBuffer),
		ThumbDir:       d.ThumbDir,
		CompareDir:     d.CompareDir,
		ModelCacheDir:  d.ModelCacheDir,
		ModelConverter: d.ModelConverter,
	}
}
