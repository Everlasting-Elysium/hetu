//go:build unix

package mediaproc

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup makes cmd the leader of a new process group and, on
// context cancel/timeout, kills the entire group (negative pid). External tools
// like soffice fork children (soffice.bin); killing only the direct child would
// orphan the grandchild, which the group kill reaps too. This overrides the
// single-process kill that exec.CommandContext installs by default.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
