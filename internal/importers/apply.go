package importers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// keySourceURL is the extracted-layer annotation key holding a migrated asset's
// origin URL (JSON string). It lives in the extracted layer so it never shadows
// a user's in-hetu manual edits.
const keySourceURL = "source.url"

// applyMetadata maps an item's portable metadata onto the stored asset: rating,
// primary folder (with ancestors), hierarchical tags (deduped by name), the note
// (extracted-layer caption), and the source URL (extracted-layer "source.url").
// It is best-effort ("尽力而为", issue #57): the asset is already indexed, so a
// per-field failure (e.g. a concurrent tag/folder create race) is logged and
// skipped rather than failing the whole item and leaving it mis-counted. Folders
// and tags dedupe on their unique keys and annotations upsert in place, so it is
// idempotent on re-import.
func (s *Service) applyMetadata(ctx context.Context, asset domain.Asset, item ImportItem) {
	if err := s.loadCaches(ctx); err != nil {
		s.warn(ctx, asset.ID, "load caches", err)
		return
	}
	ids := []domain.AssetID{asset.ID}
	if item.Rating > 0 {
		if err := s.k.Store.BatchUpdateRating(ctx, s.owner, ids, clampRating(item.Rating)); err != nil {
			s.warn(ctx, asset.ID, "rating", err)
		}
	}
	if len(item.Folders) > 0 {
		if folderID, err := s.ensureFolderPath(ctx, item.Folders[0]); err != nil {
			s.warn(ctx, asset.ID, "folder", err)
		} else if folderID != "" {
			if err := s.k.Store.BatchMoveToFolder(ctx, s.owner, ids, folderID); err != nil {
				s.warn(ctx, asset.ID, "folder assign", err)
			}
		}
	}
	if len(item.Tags) > 0 {
		if tagIDs, err := s.ensureTags(ctx, item.Tags); err != nil {
			s.warn(ctx, asset.ID, "tags", err)
		} else if len(tagIDs) > 0 {
			if err := s.k.Store.BatchAddTags(ctx, s.owner, ids, tagIDs); err != nil {
				s.warn(ctx, asset.ID, "tag attach", err)
			}
		}
	}
	s.applyAnnotations(ctx, asset.ID, item)
}

// applyAnnotations writes the note and source URL as extracted-layer
// annotations. The note uses the caption key so it surfaces as the FTS
// description (manual > ai > extracted) while never overwriting a caption the
// user typed in hetu; both are machine-imported, so neither enters the manual
// layer (see docs/ai-and-3d.md layering rules).
func (s *Service) applyAnnotations(ctx context.Context, id domain.AssetID, item ImportItem) {
	if note := strings.TrimSpace(item.Note); note != "" {
		if err := s.putStringAnnotation(ctx, id, domain.LayerExtracted, domain.KeyCaption, note); err != nil {
			s.warn(ctx, id, "note", err)
		}
	}
	if url := strings.TrimSpace(item.SourceURL); url != "" {
		if err := s.putStringAnnotation(ctx, id, domain.LayerExtracted, keySourceURL, url); err != nil {
			s.warn(ctx, id, "source url", err)
		}
	}
}

// warn logs a best-effort metadata mapping failure (the asset itself is indexed).
func (s *Service) warn(ctx context.Context, id domain.AssetID, field string, err error) {
	s.k.Log.WarnContext(ctx, "import: metadata field not applied",
		slog.String("asset", id.String()), slog.String("field", field), slog.Any("err", err))
}

func (s *Service) putStringAnnotation(ctx context.Context, id domain.AssetID, layer domain.Layer, key, val string) error {
	enc, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal annotation %q: %w", key, err)
	}
	return s.k.Store.UpsertAnnotation(ctx, s.owner, domain.Annotation{
		AssetID: id, Layer: layer, Key: key, Value: string(enc),
	})
}

// loadCaches populates the tag/folder dedup caches once per Service.
func (s *Service) loadCaches(ctx context.Context) error {
	if s.cachesLoaded {
		return nil
	}
	tags, err := s.k.Store.ListTags(ctx, s.owner)
	if err != nil {
		return fmt.Errorf("load tags: %w", err)
	}
	s.tagsByName = make(map[string]string, len(tags))
	for _, t := range tags {
		s.tagsByName[t.Name] = t.ID.String()
	}
	folders, err := s.k.Store.ListFolders(ctx, s.owner)
	if err != nil {
		return fmt.Errorf("load folders: %w", err)
	}
	s.foldersByPath = make(map[string]string, len(folders))
	for _, f := range folders {
		s.foldersByPath[f.Path] = f.ID.String()
	}
	s.cachesLoaded = true
	return nil
}

// ensureFolderPath ensures every folder in the root→leaf path exists (creating
// missing levels parent-linked, deduped by full path) and returns the leaf id.
func (s *Service) ensureFolderPath(ctx context.Context, segs NamePath) (string, error) {
	segs = cleanSegments(segs)
	if len(segs) == 0 {
		return "", nil
	}
	parentID, path, leafID := "", "", ""
	for i, name := range segs {
		if i == 0 {
			path = name
		} else {
			path += "/" + name
		}
		if id, ok := s.foldersByPath[path]; ok {
			parentID, leafID = id, id
			continue
		}
		raw := uuid.Must(uuid.NewV7()).String()
		fid, err := domain.NewFolderID(raw)
		if err != nil {
			return "", err
		}
		if err := s.k.Store.CreateFolder(ctx, domain.Folder{
			ID: fid, Owner: s.owner, ParentID: parentID, Name: name, Path: path,
		}); err != nil {
			return "", fmt.Errorf("create folder %q: %w", path, err)
		}
		s.foldersByPath[path] = raw
		parentID, leafID = raw, raw
	}
	return leafID, nil
}

// ensureTags ensures every tag path exists (deduped by name — tags are globally
// unique per owner, so equal leaf names across branches merge) and returns the
// distinct leaf tag ids to attach.
func (s *Service) ensureTags(ctx context.Context, paths []NamePath) ([]domain.TagID, error) {
	var leaves []domain.TagID
	seen := make(map[string]bool)
	for _, p := range paths {
		segs := cleanSegments(p)
		if len(segs) == 0 {
			continue
		}
		parentID, leafID := "", ""
		for _, name := range segs {
			id, err := s.resolveTag(ctx, name, parentID)
			if err != nil {
				return nil, err
			}
			parentID, leafID = id, id
		}
		if leafID == "" || seen[leafID] {
			continue
		}
		seen[leafID] = true
		tid, err := domain.NewTagID(leafID)
		if err != nil {
			return nil, err
		}
		leaves = append(leaves, tid)
	}
	return leaves, nil
}

// resolveTag returns the id of the tag named name, creating it (parent-linked,
// best effort) when absent. Dedup is by name because of the (owner, name) unique
// index; parentID is applied only on creation.
func (s *Service) resolveTag(ctx context.Context, name, parentID string) (string, error) {
	if id, ok := s.tagsByName[name]; ok {
		return id, nil
	}
	raw := uuid.Must(uuid.NewV7()).String()
	tid, err := domain.NewTagID(raw)
	if err != nil {
		return "", err
	}
	if err := s.k.Store.CreateTag(ctx, domain.Tag{
		ID: tid, Owner: s.owner, ParentID: parentID, Name: name,
	}); err != nil {
		return "", fmt.Errorf("create tag %q: %w", name, err)
	}
	s.tagsByName[name] = raw
	return raw, nil
}

// cleanSegments trims each segment and drops empties so a stray separator or
// blank name never yields an empty tag/folder.
func cleanSegments(segs NamePath) NamePath {
	out := make(NamePath, 0, len(segs))
	for _, s := range segs {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func clampRating(r int) int {
	switch {
	case r < 0:
		return 0
	case r > 5:
		return 5
	default:
		return r
	}
}
