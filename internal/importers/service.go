package importers

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/index"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/storage/fs"
)

// Mode selects how an imported file is placed relative to the library.
type Mode string

const (
	// ModeIndex registers the file in place (no copy); external paths resolve
	// through the fs provider. Default and the only safe mode for migration.
	ModeIndex Mode = "index"
	// ModeCopy copies the file into the library dir, then indexes it (local).
	ModeCopy Mode = "copy"
	// ModeMove moves the file into the library dir, then indexes it (local).
	// It deletes the original, so it is forbidden for migration sources.
	ModeMove Mode = "move"
)

// Conflict selects how a content duplicate (same hash, different path) is handled.
type Conflict string

const (
	// ConflictKeepBoth imports regardless of existing content (default).
	ConflictKeepBoth Conflict = "keep-both"
	// ConflictSkip skips an item whose content hash already exists.
	ConflictSkip Conflict = "skip"
)

// Options configures an import run.
type Options struct {
	Mode       Mode     // placement mode; empty defaults to ModeIndex
	DestSubdir string   // copy/move: destination subdir under the library root
	Conflict   Conflict // duplicate policy; empty defaults to ConflictKeepBoth
}

func (o Options) mode() Mode {
	if o.Mode == "" {
		return ModeIndex
	}
	return o.Mode
}

// Service ingests files — loose paths (#18) or migration items (#57) — through
// the index pipeline and maps portable metadata onto hetu's model. It caches
// the owner's tags (by name) and folders (by path) for the duration of one
// import run to dedupe without a query per item, so a Service is single-run:
// create a fresh one per import operation.
type Service struct {
	k     *kernel.Kernel
	ix    *index.Indexer
	owner domain.OwnerID

	tagsByName    map[string]string // tag name → id (tags are unique by name)
	foldersByPath map[string]string // folder path → id (folders unique by path)
	cachesLoaded  bool
}

// New returns an import service bound to a kernel and owner.
func New(k *kernel.Kernel, owner domain.OwnerID) *Service {
	return &Service{k: k, ix: index.New(k, owner), owner: owner}
}

// ImportPath imports a single loose file at absPath (#18) with no source
// metadata. Returns the stored asset and whether it was skipped by conflict.
func (s *Service) ImportPath(ctx context.Context, absPath string, opt Options) (domain.Asset, bool, error) {
	return s.ImportItem(ctx, ImportItem{AbsPath: absPath, Name: filepath.Base(absPath)}, opt)
}

// ImportItem places item's file per opt.Mode, indexes it, then applies its
// metadata (rating, folders, tags, note, source URL). It is idempotent on
// re-import (natural-key upsert) and returns skipped=true when opt.Conflict is
// ConflictSkip and the content already exists under a different path.
func (s *Service) ImportItem(ctx context.Context, item ImportItem, opt Options) (domain.Asset, bool, error) {
	canonical, err := canonicalPath(item.AbsPath)
	if err != nil {
		return domain.Asset{}, false, err
	}
	if opt.Conflict == ConflictSkip {
		dup, err := s.contentExists(ctx, canonical)
		if err != nil {
			return domain.Asset{}, false, err
		}
		if dup {
			s.k.Log.InfoContext(ctx, "import skip: content exists", "path", canonical)
			return domain.Asset{}, true, nil
		}
	}

	providerName, entry, cleanup, err := s.place(ctx, canonical, item, opt)
	if err != nil {
		return domain.Asset{}, false, err
	}
	asset, err := s.ix.IndexFile(ctx, providerName, entry)
	if err != nil {
		cleanup(ctx) // roll back a copy/move destination on index failure
		return domain.Asset{}, false, fmt.Errorf("import %q: %w", item.Name, err)
	}
	// Metadata mapping is best-effort: the asset is indexed, so a per-field
	// failure is logged inside applyMetadata, not surfaced as an import failure.
	s.applyMetadata(ctx, asset, item)
	// Move deletes the original only after a fully successful import; a failed
	// delete leaves an orphan source (safe) and is logged, never rolled back.
	if opt.mode() == ModeMove {
		if err := removePath(canonical); err != nil {
			s.k.Log.WarnContext(ctx, "move: delete original failed", "path", canonical, "err", err)
		}
	}
	return asset, false, nil
}

// contentExists reports whether a live asset already has the content hash of the
// file at canonical (used by ConflictSkip).
func (s *Service) contentExists(ctx context.Context, canonical string) (bool, error) {
	prov, ok := s.k.Storage.Get(fs.ProviderName)
	if !ok {
		return false, fmt.Errorf("content check: provider %q: %w", fs.ProviderName, domain.ErrNotFound)
	}
	hash, err := hashFile(ctx, prov, canonical)
	if err != nil {
		return false, err
	}
	existing, err := s.k.Store.ListAssetsByHash(ctx, s.owner, hash)
	if err != nil {
		return false, err
	}
	return len(existing) > 0, nil
}
