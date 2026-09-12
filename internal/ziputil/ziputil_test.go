package ziputil_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/ziputil"
)

// readZip parses buf as a zip archive and returns its entries as name→content.
func readZip(t *testing.T, buf []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	out := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(data)
	}
	return out
}

// discardLog returns a logger that drops output — Stream logs per-item skips.
func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// writeFile creates name under dir with body; the local provider opens by the
// same relative name.
func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestStreamHappy: multiple items each land as a named entry carrying its bytes.
func TestStreamHappy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "one.png", "ONE")
	writeFile(t, dir, "two.png", "TWO")
	prov := local.New(dir)

	var buf bytes.Buffer
	ziputil.Stream(context.Background(), discardLog(), &buf, []ziputil.Item{
		{Provider: prov, Path: "one.png", Name: "one.png"},
		{Provider: prov, Path: "two.png", Name: "two.png"},
	})

	entries := readZip(t, buf.Bytes())
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2: %v", len(entries), entries)
	}
	if entries["one.png"] != "ONE" || entries["two.png"] != "TWO" {
		t.Errorf("entries = %v, want one.png=ONE two.png=TWO", entries)
	}
}

// TestStreamDedupsNames: two items sharing an entry name → the second is
// suffixed " (2)" before the extension so neither is lost.
func TestStreamDedupsNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.png", "ONE")
	writeFile(t, dir, "b.png", "TWO")
	prov := local.New(dir)

	var buf bytes.Buffer
	ziputil.Stream(context.Background(), discardLog(), &buf, []ziputil.Item{
		{Provider: prov, Path: "a.png", Name: "same.png"},
		{Provider: prov, Path: "b.png", Name: "same.png"},
	})

	entries := readZip(t, buf.Bytes())
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2: %v", len(entries), entries)
	}
	if entries["same.png"] != "ONE" {
		t.Errorf("same.png = %q, want ONE", entries["same.png"])
	}
	if entries["same (2).png"] != "TWO" {
		t.Errorf("'same (2).png' = %q, want TWO", entries["same (2).png"])
	}
}

// TestStreamSkipsOpenFailure: an item whose backing file is missing is skipped
// (open error) without aborting the stream, and — because only successfully-
// opened items consume a name slot — a later good item keeps the un-suffixed
// name rather than being pushed to " (2)".
func TestStreamSkipsOpenFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.png", "GOOD")
	prov := local.New(dir)

	var buf bytes.Buffer
	ziputil.Stream(context.Background(), discardLog(), &buf, []ziputil.Item{
		{Provider: prov, Path: "missing.png", Name: "x.png"},
		{Provider: prov, Path: "good.png", Name: "x.png"},
	})

	entries := readZip(t, buf.Bytes())
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (missing skipped): %v", len(entries), entries)
	}
	if entries["x.png"] != "GOOD" {
		t.Errorf("entries = %v, want x.png=GOOD (skip consumes no name slot)", entries)
	}
}
