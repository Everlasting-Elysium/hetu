package mediaproc_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
)

func TestTempCopy(t *testing.T) {
	// Given a reader whose cursor is not at the start.
	want := []byte("river map bytes")
	src := bytes.NewReader(want)
	if _, err := src.Read(make([]byte, 4)); err != nil {
		t.Fatalf("prime read: %v", err)
	}

	// When copying it to a temp file.
	path, cleanup, err := mediaproc.TempCopy(src, ".bin")
	if err != nil {
		t.Fatalf("TempCopy: %v", err)
	}
	defer cleanup()

	// Then the full content is copied (seek-to-start) with the given suffix.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
	if !strings.HasSuffix(path, ".bin") {
		t.Fatalf("path %q missing .bin suffix", path)
	}

	// And cleanup removes the file.
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file still present after cleanup: %v", err)
	}
}

func TestRunCapturesStdout(t *testing.T) {
	sh := shOrSkip(t)
	out, err := mediaproc.Run(context.Background(), 5*time.Second, sh, "-c", "printf hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "hello" {
		t.Fatalf("stdout = %q, want %q", out, "hello")
	}
}

func TestRunFoldsStderrIntoError(t *testing.T) {
	sh := shOrSkip(t)
	_, err := mediaproc.Run(context.Background(), 5*time.Second, sh, "-c", "echo boom >&2; exit 3")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error %q does not contain stderr %q", err, "boom")
	}
}

func TestRunMissingBinary(t *testing.T) {
	_, err := mediaproc.Run(context.Background(), time.Second, "hetu-nonexistent-binary-xyz")
	if err == nil {
		t.Fatal("want error for missing binary, got nil")
	}
}

func TestRunTimeout(t *testing.T) {
	sh := shOrSkip(t)
	start := time.Now()
	_, err := mediaproc.Run(context.Background(), 50*time.Millisecond, sh, "-c", "sleep 5")
	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Run did not honor timeout: took %v", elapsed)
	}
}

func shOrSkip(t *testing.T) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	return sh
}
