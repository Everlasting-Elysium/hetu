package wallpaper

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// getZip fetches a zip response and returns its entries as name→content.
func (e *testEnv) getZip(path string) map[string]string {
	e.t.Helper()
	resp := e.get(path)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		e.t.Errorf("zip Content-Type = %q, want application/zip", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		e.t.Fatalf("read zip: %v", err)
	}
	out := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			e.t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			e.t.Fatal(err)
		}
		out[f.Name] = string(data)
	}
	return out
}

// TestDownloadZipHappy covers a multi-asset bundle: each entry is named by the
// asset and carries its bytes.
func TestDownloadZipHappy(t *testing.T) {
	e := newTestEnv(t)
	a1 := e.add(seed{id: "a1", name: "one.png", content: []byte("ONE")})
	a2 := e.add(seed{id: "a2", name: "two.png", content: []byte("TWO")})

	entries := e.getZip("/api/wallpaper/download/zip?ids=" + a1.String() + "," + a2.String())
	if len(entries) != 2 {
		t.Fatalf("zip has %d entries, want 2: %v", len(entries), entries)
	}
	if entries["one.png"] != "ONE" || entries["two.png"] != "TWO" {
		t.Errorf("zip entries = %v, want one.png=ONE two.png=TWO", entries)
	}
}

// TestDownloadZipSkipsMissing covers a request mixing a valid and an unknown id:
// the unknown one is dropped, the valid one still ships.
func TestDownloadZipSkipsMissing(t *testing.T) {
	e := newTestEnv(t)
	a1 := e.add(seed{id: "a1", name: "one.png", content: []byte("ONE")})

	entries := e.getZip("/api/wallpaper/download/zip?ids=" + a1.String() + ",does-not-exist")
	if len(entries) != 1 || entries["one.png"] != "ONE" {
		t.Errorf("zip = %v, want just one.png=ONE", entries)
	}
}

// TestDownloadZipErrors covers the request-validation branches: empty ids and a
// missing ids param are 400; an all-unresolvable list is 404.
func TestDownloadZipErrors(t *testing.T) {
	e := newTestEnv(t)
	e.add(seed{id: "a1", name: "one.png", content: []byte("ONE")})

	cases := []struct {
		name, path string
		want       int
	}{
		{"empty ids", "/api/wallpaper/download/zip?ids=", http.StatusBadRequest},
		{"no ids param", "/api/wallpaper/download/zip", http.StatusBadRequest},
		{"all missing", "/api/wallpaper/download/zip?ids=nope1,nope2", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := e.get(tc.path)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.want {
				t.Errorf("%s = %d, want %d", tc.path, resp.StatusCode, tc.want)
			}
		})
	}
}

// TestDownloadZipOverCap covers the maxZipItems guard.
func TestDownloadZipOverCap(t *testing.T) {
	e := newTestEnv(t)
	ids := make([]string, 0, maxZipItems+1)
	for i := range maxZipItems + 1 {
		aid := e.add(seed{id: fmt.Sprintf("a%d", i), name: fmt.Sprintf("w%d.png", i), content: []byte("x")})
		ids = append(ids, aid.String())
	}
	resp := e.get("/api/wallpaper/download/zip?ids=" + strings.Join(ids, ","))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("over-cap zip = %d, want 400", resp.StatusCode)
	}
}

// TestDownloadZipNameConflict covers the de-dup suffixing: two assets whose
// download name collides get " (2)" appended before the extension.
func TestDownloadZipNameConflict(t *testing.T) {
	e := newTestEnv(t)
	a1 := e.add(seed{id: "a1", name: "a1.png", content: []byte("ONE"), display: "same.png"})
	a2 := e.add(seed{id: "a2", name: "a2.png", content: []byte("TWO"), display: "same.png"})

	entries := e.getZip("/api/wallpaper/download/zip?ids=" + a1.String() + "," + a2.String())
	if len(entries) != 2 {
		t.Fatalf("zip has %d entries, want 2: %v", len(entries), entries)
	}
	if _, ok := entries["same.png"]; !ok {
		t.Errorf("missing first entry same.png: %v", entries)
	}
	if _, ok := entries["same (2).png"]; !ok {
		t.Errorf("missing de-duped entry 'same (2).png': %v", entries)
	}
}
