package domain

import (
	"fmt"
	"path"
	"strings"
	"time"
)

// ManagedDirName is the reserved library subdirectory that holds hetu-managed
// files (currently version copies). The indexer skips it during scans and the
// NAS plugin hides it from listings, so managed internals never surface as user
// assets. It lives inside the library root because version bytes belong to the
// same storage provider as the assets they version.
const ManagedDirName = ".hetu"

// versionsSubdir is the ManagedDirName subtree under which version copies live.
const versionsSubdir = "versions"

// VersionID identifies a single stored revision of an asset.
type VersionID struct{ raw string }

// NewVersionID parses s into a VersionID.
func NewVersionID(s string) (VersionID, error) {
	if s == "" {
		return VersionID{}, fmt.Errorf("version id: %w", ErrEmptyID)
	}
	return VersionID{raw: s}, nil
}

// String returns the raw version id.
func (id VersionID) String() string { return id.raw }

// AssetVersion is one stored revision of an asset. Versions of an asset are
// grouped under a single AssetID; the asset's current_version_id points at the
// active one. version 1 is the original in-place indexed file (its StoragePath
// is the anchor path, outside ManagedDirName); versions 2+ are copies uploaded
// through the API and stored under ManagedDirName. Reading/thumbnailing an asset
// resolves through its current version (see kernel.Store).
type AssetVersion struct {
	ID          VersionID
	AssetID     AssetID
	Owner       OwnerID
	VersionNo   int
	Provider    string
	StoragePath string
	Hash        string // SHA-256 of this version's content
	Size        int64
	ThumbPath   string
	Width       int
	Height      int
	Note        string
	CreatedAt   time.Time
}

// VersionStoragePath returns the provider-relative path for a managed version
// copy: <ManagedDirName>/versions/<assetID>/<versionID>/<filename>. Keying the
// directory by versionID (not version number) decouples the on-disk layout from
// version-number allocation, which happens later inside the DB transaction.
func VersionStoragePath(assetID AssetID, versionID VersionID, filename string) string {
	return path.Join(ManagedDirName, versionsSubdir, assetID.String(), versionID.String(), path.Base(filename))
}

// VersionsAssetDir returns the managed directory holding every version copy for
// an asset: <ManagedDirName>/versions/<assetID>. Used to purge on hard delete.
func VersionsAssetDir(assetID AssetID) string {
	return path.Join(ManagedDirName, versionsSubdir, assetID.String())
}

// IsManagedPath reports whether a provider-relative path is under the managed
// versions tree. Deletion removes managed copies but must never delete a user's
// in-place original (version 1), whose path lies outside this tree.
func IsManagedPath(p string) bool {
	return strings.HasPrefix(path.Clean(p), ManagedDirName+"/"+versionsSubdir+"/")
}
