package wallpaper

import (
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// wallpaperDTO is the public, read-only wire form of a wallpaper asset. It
// deliberately omits storage_path/provider/folder_id/name so no internal
// filesystem path, storage backend, organization, or original filename leaks
// through the anonymous endpoints (issue #114). ThumbURL/DownloadURL are
// relative paths into this same plugin.
type wallpaperDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Size        int64  `json:"size"`
	Rating      int    `json:"rating"`
	ThumbURL    string `json:"thumb_url"`
	DownloadURL string `json:"download_url"`
}

func toWallpaperDTO(a domain.Asset) wallpaperDTO {
	id := a.ID.String()
	return wallpaperDTO{
		ID:          id,
		Kind:        string(a.Kind),
		Width:       a.Width,
		Height:      a.Height,
		Size:        a.Size,
		Rating:      a.Rating,
		ThumbURL:    fmt.Sprintf("/api/wallpaper/%s/thumb", id),
		DownloadURL: fmt.Sprintf("/api/wallpaper/%s/download", id),
	}
}

func toWallpaperDTOs(assets []domain.Asset) []wallpaperDTO {
	out := make([]wallpaperDTO, 0, len(assets))
	for _, a := range assets {
		out = append(out, toWallpaperDTO(a))
	}
	return out
}
