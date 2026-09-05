// Package mediaproc runs external media tools (ffmpeg, ffprobe, pdftoppm,
// mutool) as subprocesses for the video and document asset handlers. Delegating
// heavy media work to sidecar binaries keeps the kernel CGO-free; the binaries
// may be absent, in which case the handlers degrade gracefully.
package mediaproc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TempCopy writes src, from its start, to a temporary file with the given
// suffix and returns the path plus a cleanup func the caller must defer.
// External media tools need a seekable file path, not a stream.
func TempCopy(src io.ReadSeeker, suffix string) (string, func(), error) {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", nil, fmt.Errorf("seek source: %w", err)
	}
	f, err := os.CreateTemp("", "hetu-media-*"+suffix)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, fmt.Errorf("copy to temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}
	return f.Name(), cleanup, nil
}

// Run executes name with args under timeout and returns its stdout. stderr is
// captured and folded into the returned error on failure so callers can log a
// meaningful reason for a degraded (thumbnail-less) asset.
func Run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, fmt.Errorf("run %s: %w", name, err)
		}
		return nil, fmt.Errorf("run %s: %w: %s", name, err, msg)
	}
	return stdout.Bytes(), nil
}
