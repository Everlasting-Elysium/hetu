package domain

import "time"

// ExtractedMetadata holds file-embedded metadata (EXIF/IPTC/XMP) extracted by
// an AssetHandler. Each entry in Annotations becomes one row in the annotations
// table under the extracted layer.
type ExtractedMetadata struct {
	// Annotations maps keys (e.g., "exif.camera_make", "iptc.keywords") to
	// their JSON-serializable values. Each entry becomes one annotations row.
	Annotations map[string]any

	// DateTime is the embedded capture time (EXIF DateTimeOriginal). Zero
	// means no embedded time found; when non-zero it takes priority over the
	// filesystem modification time for asset.created_at.
	DateTime time.Time

	// Keywords are IPTC/XMP keywords stored as extracted-layer tag
	// suggestions. They do NOT auto-promote into the manual layer.
	Keywords []string
}
