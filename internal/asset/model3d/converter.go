package model3d

import (
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// Converter converts a 3D model to GLB. It is a type alias for
// kernel.ModelConverter: the abstraction is defined in the kernel package so the
// kernel can hold it without importing model3d (which imports the kernel), and
// aliased here where the concrete backends live so the package reads naturally.
type Converter = kernel.ModelConverter

// Compile-time proof that both backends satisfy the Converter contract.
var (
	_ Converter = (*BlenderConverter)(nil)
	_ Converter = (*AssimpConverter)(nil)
)

// NewConverter builds the 3D→GLB converter selected by backend:
//
//   - "blender": the Blender headless sidecar at blenderAddr.
//   - "assimp":  the assimp CLI subprocess; errors when assimp is not on PATH.
//   - "":        auto-detect — assimp when on PATH, else Blender when
//     blenderAddr is set, else no converter (returns nil, nil so conversion
//     degrades gracefully instead of failing startup).
//
// Only an unknown backend name is a configuration error; every other case
// either returns a converter or a nil converter.
func NewConverter(backend, blenderAddr string) (Converter, error) {
	switch backend {
	case "blender":
		return &BlenderConverter{addr: blenderAddr}, nil
	case "assimp":
		if !assimpAvailable() {
			return nil, fmt.Errorf("model3d: assimp backend requested but %q not found on PATH", assimpBinary)
		}
		return &AssimpConverter{}, nil
	case "":
		if assimpAvailable() {
			return &AssimpConverter{}, nil
		}
		if blenderAddr != "" {
			return &BlenderConverter{addr: blenderAddr}, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("model3d: unknown converter backend %q", backend)
	}
}
