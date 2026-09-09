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
	// ConflictMerge folds an item's metadata into the existing content
	// duplicate (tags/note added; rating/folder only when the target is unset)
	// and lands neither a second physical file nor a new asset row.
	ConflictMerge Conflict = "merge"
)

// Outcome reports how ImportItem resolved one item: a new asset was indexed
// (imported), a content duplicate was left untouched (skipped), or an item's
// metadata was folded into an existing duplicate (merged).
type Outcome string

const (
	OutcomeImported Outcome = "imported"
	OutcomeSkipped  Outcome = "skipped"
	OutcomeMerged   Outcome = "merged"
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
// metadata. Returns the stored asset and the import Outcome (see ImportItem).
func (s *Service) ImportPath(ctx context.Context, absPath string, opt Options) (domain.Asset, Outcome, error) {
	return s.ImportItem(ctx, ImportItem{AbsPath: absPath, Name: filepath.Base(absPath)}, opt)
}

// ImportItem places item's file per opt.Mode, indexes it, then applies its
// metadata (rating, folders, tags, note, source URL). It is idempotent on
// re-import (natural-key upsert). When opt.Conflict is ConflictSkip or
// ConflictMerge and the content already exists under a different path, it
// short-circuits before place/index — so move never deletes the source and copy
// never lands a second physical file: skip returns an empty asset + OutcomeSkipped,
// merge folds item's metadata into the existing asset and returns it +
// OutcomeMerged. Otherwise it returns the new asset + OutcomeImported.
func (s *Service) ImportItem(ctx context.Context, item ImportItem, opt Options) (domain.Asset, Outcome, error) {
	canonical, err := canonicalPath(item.AbsPath)
	if err != nil {
		return domain.Asset{}, "", err
	}
	if opt.Conflict == ConflictSkip || opt.Conflict == ConflictMerge {
		existing, err := s.findDuplicate(ctx, canonical)
		if err != nil {
			return domain.Asset{}, "", err
		}
		if existing != nil {
			if opt.Conflict == ConflictMerge {
				s.k.Log.InfoContext(ctx, "import merge: content exists", "path", canonical)
				s.applyMetadata(ctx, *existing, item)
				return *existing, OutcomeMerged, nil
			}
			s.k.Log.InfoContext(ctx, "import skip: content exists", "path", canonical)
			return domain.Asset{}, OutcomeSkipped, nil
		}
	}

	providerName, entry, cleanup, err := s.place(ctx, canonical, item, opt)
	if err != nil {
		return domain.Asset{}, "", err
	}
	asset, err := s.ix.IndexFile(ctx, providerName, entry)
	if err != nil {
		cleanup(ctx) // roll back a copy/move destination on index failure
		return domain.Asset{}, "", fmt.Errorf("import %q: %w", item.Name, err)
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
	return asset, OutcomeImported, nil
}

// findDuplicate returns the first (oldest) live asset already holding the
// content hash of the file at canonical, or nil when none exists. It backs both
// ConflictSkip (skip the re-encountered file) and ConflictMerge (fold new
// metadata into that asset); both reach identical bytes via a different path.
func (s *Service) findDuplicate(ctx context.Context, canonical string) (*domain.Asset, error) {
	prov, ok := s.k.Storage.Get(fs.ProviderName)
	if !ok {
		return nil, fmt.Errorf("content check: provider %q: %w", fs.ProviderName, domain.ErrNotFound)
	}
	hash, err := hashFile(ctx, prov, canonical)
	if err != nil {
		return nil, err
	}
	existing, err := s.k.Store.ListAssetsByHash(ctx, s.owner, hash)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		return nil, nil
	}
	return &existing[0], nil
}
