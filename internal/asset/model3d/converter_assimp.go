package model3d

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// assimpBinary is the assimp CLI looked up on PATH. assimp is a pure-native
// (no-Blender, no-GPU) importer/exporter covering OBJ/FBX/STL/PLY/glTF and more,
// so it fits self-hosted hosts that cannot run a Blender sidecar.
const assimpBinary = "assimp"

// assimpTimeout bounds a single assimp export subprocess. A malformed or huge
// mesh must not wedge a viewer request indefinitely.
const assimpTimeout = 120 * time.Second

// AssimpConverter converts 3D models to GLB by shelling out to the assimp CLI
// (`assimp export <in> <out.glb>`). It has no configuration: availability is
// decided once at construction (see NewConverter / assimpAvailable).
type AssimpConverter struct{}

// ConvertToGLB writes src to a temp file named with ext (assimp selects the
// importer from the extension), runs `assimp export` into a temp GLB, then
// streams that GLB into w. Both temp files live in a per-call temp dir removed
// on return. A non-zero assimp exit surfaces verbatim (with captured output) so
// a real conversion error reaches the viewer instead of a silent empty model.
func (a *AssimpConverter) ConvertToGLB(ctx context.Context, ext string, src io.ReadSeeker, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, assimpTimeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "hetu-assimp-*")
	if err != nil {
		return fmt.Errorf("model3d: assimp temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	inPath := filepath.Join(dir, "input."+ext)
	outPath := filepath.Join(dir, "output.glb")
	if err := writeTempInput(inPath, src); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, assimpBinary, "export", inPath, outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("model3d: assimp export: %w: %s", err, out)
	}

	glb, err := os.Open(outPath)
	if err != nil {
		return fmt.Errorf("model3d: open assimp output: %w", err)
	}
	defer func() { _ = glb.Close() }()
	if _, err := io.Copy(w, glb); err != nil {
		return fmt.Errorf("model3d: copy assimp output: %w", err)
	}
	return nil
}

// writeTempInput copies src into a new file at path, closing it before return so
// the assimp subprocess reads a fully-flushed input.
func writeTempInput(path string, src io.ReadSeeker) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("model3d: create assimp input: %w", err)
	}
	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()
		return fmt.Errorf("model3d: write assimp input: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("model3d: close assimp input: %w", err)
	}
	return nil
}

// assimpAvailable reports whether the assimp CLI is resolvable on PATH.
func assimpAvailable() bool {
	_, err := exec.LookPath(assimpBinary)
	return err == nil
}
