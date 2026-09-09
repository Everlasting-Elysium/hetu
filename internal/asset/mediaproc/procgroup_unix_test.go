//go:build unix

package mediaproc_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
)

// TestRunKillsProcessGroup proves a timeout reaps the whole process group, not
// just the direct child: a shell backgrounds a long sleep and waits; when Run
// times out, the orphaned sleep must also die. Without the process-group kill it
// would be reparented to init and survive.
func TestRunKillsProcessGroup(t *testing.T) {
	sh := shOrSkip(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	// Background a long sleep, record its PID, then wait so the parent stays
	// alive until the timeout fires.
	script := fmt.Sprintf("sleep 60 & echo $! > %s; wait", pidFile)
	_, err := mediaproc.Run(context.Background(), 300*time.Millisecond, sh, "-c", script)
	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
	pid := readPID(t, pidFile)
	if pid <= 0 {
		t.Skip("child pid not recorded in time")
	}
	// The backgrounded sleep must be reaped by the group kill within a moment.
	for range 100 {
		if syscall.Kill(pid, 0) != nil { // ESRCH: process gone
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL) // best-effort cleanup before failing
	t.Fatalf("orphaned child %d survived the process-group kill", pid)
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}
