package rclone

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// testContent is 1024 bytes where each byte equals its index mod 256. Using
// distinct (non-repeating) content means a Range/offset bug cannot be masked
// by identical bytes at different offsets.
func testContent() []byte {
	b := make([]byte, 1024)
	for i := range b {
		b[i] = byte(i % 256)
	}
	return b
}

// fakeServer records what the last file-serve request received so tests can
// assert the Range header and Basic-auth were sent correctly.
type fakeServer struct {
	*httptest.Server
	lastRange string
	lastAuth  string
}

// newFakeRclone returns a server mimicking the rclone RC daemon
// (operations/list, operations/stat) plus the --rc-serve file server.
func newFakeRclone(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	content := testContent()
	mux := http.NewServeMux()

	mux.HandleFunc("POST /operations/list", func(w http.ResponseWriter, r *http.Request) {
		var req rcListReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
			return
		}
		if req.Remote == "boom" {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "directory not found"})
			return
		}
		resp := rcListResp{List: []rcItem{
			{Name: "photo.jpg", Path: "photo.jpg", Size: 1024, ModTime: "2025-06-01T12:00:00Z", IsDir: false},
			{Name: "docs", Path: "docs", Size: 0, ModTime: "2025-06-01T10:00:00Z", IsDir: true},
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /operations/stat", func(w http.ResponseWriter, r *http.Request) {
		var req rcStatReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Remote {
		case "missing.txt":
			json.NewEncoder(w).Encode(rcStatResp{Item: nil})
		case "docs":
			json.NewEncoder(w).Encode(rcStatResp{Item: &rcItem{
				Name: "docs", Path: "docs", Size: 0, ModTime: "2025-06-01T10:00:00Z", IsDir: true,
			}})
		default:
			json.NewEncoder(w).Encode(rcStatResp{Item: &rcItem{
				Name: "photo.jpg", Path: "photo.jpg", Size: 1024,
				ModTime: "2025-06-01T12:00:00Z", IsDir: false,
			}})
		}
	})

	// Simulate --rc-serve file server with Range support; record req details.
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		fs.lastRange = r.Header.Get("Range")
		fs.lastAuth = r.Header.Get("Authorization")
		http.ServeContent(w, r, "photo.jpg", time.Time{}, strings.NewReader(string(content)))
	})

	fs.Server = httptest.NewServer(mux)
	return fs
}

func newTestProvider(srv *fakeServer, user, pass string) *Provider {
	addr := strings.TrimPrefix(srv.URL, "http://")
	return New(addr, "test:", user, pass)
}

func TestList(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	entries, err := p.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "photo.jpg" || entries[0].Size != 1024 {
		t.Errorf("entry[0] = %+v", entries[0])
	}
	if !entries[1].IsDir {
		t.Error("entry[1] should be a directory")
	}
}

func TestListError(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	_, err := p.List(context.Background(), "boom")
	if err == nil {
		t.Fatal("expected error for non-2xx list response")
	}
	if !strings.Contains(err.Error(), "directory not found") {
		t.Errorf("error should carry server message, got: %v", err)
	}
	// "not found" message should map to the domain sentinel.
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestStat(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	info, err := p.Stat(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size != 1024 || info.IsDir {
		t.Errorf("info = %+v", info)
	}
}

func TestStatNotFound(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	_, err := p.Stat(context.Background(), "missing.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestOpenReadAll(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	rc, err := p.Open(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	want := testContent()
	if len(data) != len(want) {
		t.Fatalf("ReadAll length = %d, want %d", len(data), len(want))
	}
	for i := range want {
		if data[i] != want[i] {
			t.Fatalf("byte %d = %d, want %d", i, data[i], want[i])
		}
	}
}

func TestOpenSeekReadsCorrectBytes(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	rc, err := p.Open(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	// Seek to offset 100; content[100] == 100.
	pos, err := rc.Seek(100, io.SeekStart)
	if err != nil || pos != 100 {
		t.Fatalf("Seek = (%d, %v), want (100, nil)", pos, err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(rc, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	for i := range buf {
		if buf[i] != byte(100+i) {
			t.Errorf("byte at %d = %d, want %d", 100+i, buf[i], 100+i)
		}
	}
	// Verify a Range header was actually sent (distinguishes real Range support).
	if srv.lastRange != "bytes=100-" {
		t.Errorf("Range header = %q, want %q", srv.lastRange, "bytes=100-")
	}
}

func TestOpenSeekEnd(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	rc, err := p.Open(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	pos, err := rc.Seek(-4, io.SeekEnd)
	if err != nil || pos != 1020 {
		t.Fatalf("SeekEnd = (%d, %v), want (1020, nil)", pos, err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(rc, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	for i := range buf {
		if buf[i] != byte((1020+i)%256) {
			t.Errorf("byte at %d = %d, want %d", 1020+i, buf[i], (1020+i)%256)
		}
	}
}

func TestSeekBeyondSizeReadsEOF(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	rc, err := p.Open(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	if _, err := rc.Seek(2048, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	n, err := rc.Read(make([]byte, 8))
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("Read past size = (%d, %v), want (0, EOF)", n, err)
	}
}

func TestSeekInvalid(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	rc, err := p.Open(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	if _, err := rc.Seek(0, 99); err == nil {
		t.Error("expected error for invalid whence")
	}
	if _, err := rc.Seek(-1, io.SeekStart); err == nil {
		t.Error("expected error for negative offset")
	}
}

func TestReadAfterClose(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	rc, err := p.Open(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := rc.Read(make([]byte, 8)); err == nil {
		t.Error("expected error reading after Close")
	}
}

func TestOpenDirectoryFails(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "", "")

	if _, err := p.Open(context.Background(), "docs"); err == nil {
		t.Error("expected error opening a directory")
	}
}

func TestBasicAuthSent(t *testing.T) {
	srv := newFakeRclone(t)
	defer srv.Close()
	p := newTestProvider(srv, "admin", "secret")

	rc, err := p.Open(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	if _, err := io.ReadFull(rc, make([]byte, 8)); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if srv.lastAuth == "" || !strings.HasPrefix(srv.lastAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got %q", srv.lastAuth)
	}
}

func TestServeURLEscaping(t *testing.T) {
	p := New("localhost:5572", "remote:", "", "")
	got := p.serveURL("dir/a b?c#d.jpg")
	want := "http://localhost:5572/[remote:]/dir/a%20b%3Fc%23d.jpg"
	if got != want {
		t.Errorf("serveURL = %q, want %q", got, want)
	}
}

func TestProviderName(t *testing.T) {
	p := New("localhost:5572", "remote:", "", "")
	if p.Name() != ProviderName {
		t.Errorf("Name() = %q, want %q", p.Name(), ProviderName)
	}
}

func TestUnreachable(t *testing.T) {
	p := New("127.0.0.1:1", "remote:", "", "")

	if _, err := p.List(context.Background(), ""); err == nil {
		t.Error("expected error for unreachable server (List)")
	}
	if _, err := p.Stat(context.Background(), "file.txt"); err == nil {
		t.Error("expected error for unreachable server (Stat)")
	}
}
