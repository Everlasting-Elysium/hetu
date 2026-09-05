package domain

import "fmt"

// OwnerID identifies the owner of an asset. Single-user today; the field is
// reserved in every table so multi-user is additive later.
type OwnerID struct{ raw string }

// NewOwnerID parses s into an OwnerID.
func NewOwnerID(s string) (OwnerID, error) {
	if s == "" {
		return OwnerID{}, fmt.Errorf("owner id: %w", ErrEmptyID)
	}
	return OwnerID{raw: s}, nil
}

// String returns the raw owner id.
func (id OwnerID) String() string { return id.raw }

// AssetID identifies an indexed asset.
type AssetID struct{ raw string }

// NewAssetID parses s into an AssetID.
func NewAssetID(s string) (AssetID, error) {
	if s == "" {
		return AssetID{}, fmt.Errorf("asset id: %w", ErrEmptyID)
	}
	return AssetID{raw: s}, nil
}

// String returns the raw asset id.
func (id AssetID) String() string { return id.raw }
