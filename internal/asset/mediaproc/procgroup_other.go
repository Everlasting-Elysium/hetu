//go:build !unix

package mediaproc

import "os/exec"

// configureProcessGroup is a no-op on non-Unix platforms (the server targets
// Linux containers): exec.CommandContext's default single-process kill applies.
func configureProcessGroup(_ *exec.Cmd) {}
