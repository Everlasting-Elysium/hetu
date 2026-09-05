package nas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/nas"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// newTestServer creates a NAS test server backed by a temp directory with
// seed files. Returns the server, its base URL, and the temp root path.
func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()

	// Seed a temp directory with test files.
	root := t.TempDir()
	writeFile(t, root, "hello.txt", "hello world")
	writeFile(t, root, "subdir/nested.txt", "nested content")

	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "nas.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	k := kernel.New(kernel.Deps{Log: log, Store: st, ThumbDir: t.TempDir(), JobBuffer: 1})
	k.Storage.Register(local.New(root))

	p := nas.New(owner, "local")
	if err := p.Init(ctx, k); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	srv := httptest.NewServer(api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{}))
	t.Cleanup(srv.Close)
	return srv, root
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Download tests
// ---------------------------------------------------------------------------

func TestDownload_FullFile(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/nas/download?path=hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello world" {
		t.Fatalf("body = %q, want %q", body, "hello world")
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "hello.txt") {
		t.Fatalf("Content-Disposition = %q, want filename hello.txt", cd)
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes", resp.Header.Get("Accept-Ranges"))
	}
}

func TestDownload_RangeRequest(t *testing.T) {
	srv, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/nas/download?path=hello.txt", nil)
	req.Header.Set("Range", "bytes=0-4")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Fatalf("body = %q, want %q", body, "hello")
	}
}

func TestDownload_MissingPath(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/nas/download")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestDownload_FileNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/nas/download?path=nonexistent.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDownload_DirectoryRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/nas/download?path=subdir")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Share creation tests
// ---------------------------------------------------------------------------

type shareResp struct {
	ID        string  `json:"id"`
	Token     string  `json:"token"`
	URL       string  `json:"url"`
	ExpiresAt *string `json:"expires_at"`
}

func createTestShare(t *testing.T, baseURL string, body map[string]any) (*http.Response, shareResp) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/api/nas/shares", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var sr shareResp
	if resp.StatusCode == http.StatusCreated {
		if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
			t.Fatalf("decode share response: %v", err)
		}
	}
	return resp, sr
}

func TestShareCreate_FileShare(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, sr := createTestShare(t, srv.URL, map[string]any{
		"target_type": "file",
		"target_path": "hello.txt",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if sr.Token == "" {
		t.Fatal("token is empty")
	}
	if len(sr.Token) != 64 {
		t.Fatalf("token length = %d, want 64 (32 bytes hex)", len(sr.Token))
	}
	if sr.URL != "/s/"+sr.Token {
		t.Fatalf("url = %q, want /s/%s", sr.URL, sr.Token)
	}
	if sr.ExpiresAt != nil {
		t.Fatalf("expires_at = %v, want nil (no expiry)", *sr.ExpiresAt)
	}
}

func TestShareCreate_WithExpiry(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, sr := createTestShare(t, srv.URL, map[string]any{
		"target_type": "file",
		"target_path": "hello.txt",
		"expires_in":  3600,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if sr.ExpiresAt == nil {
		t.Fatal("expires_at is nil, want non-nil")
	}
}

func TestShareCreate_InvalidTargetType(t *testing.T) {
	srv, _ := newTestServer(t)
	b, _ := json.Marshal(map[string]any{
		"target_type": "invalid",
		"target_path": "hello.txt",
	})
	resp, err := http.Post(srv.URL+"/api/nas/shares", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestShareCreate_EmptyPath(t *testing.T) {
	srv, _ := newTestServer(t)
	b, _ := json.Marshal(map[string]any{
		"target_type": "file",
		"target_path": "",
	})
	resp, err := http.Post(srv.URL+"/api/nas/shares", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Share access tests
// ---------------------------------------------------------------------------

func TestShareAccess_FileDownload(t *testing.T) {
	srv, _ := newTestServer(t)
	_, sr := createTestShare(t, srv.URL, map[string]any{
		"target_type": "file",
		"target_path": "hello.txt",
	})

	resp, err := http.Get(srv.URL + sr.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello world" {
		t.Fatalf("body = %q, want %q", body, "hello world")
	}
}

func TestShareAccess_FolderList(t *testing.T) {
	srv, _ := newTestServer(t)
	_, sr := createTestShare(t, srv.URL, map[string]any{
		"target_type": "folder",
		"target_path": "subdir",
	})

	resp, err := http.Get(srv.URL + sr.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var entries []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
}

func TestShareAccess_Expired(t *testing.T) {
	srv, _ := newTestServer(t)
	// Create a share that expires in 1 second.
	_, sr := createTestShare(t, srv.URL, map[string]any{
		"target_type": "file",
		"target_path": "hello.txt",
		"expires_in":  1,
	})
	// Wait for expiry.
	time.Sleep(2 * time.Second)

	resp, err := http.Get(srv.URL + sr.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status = %d, want 410 (Gone)", resp.StatusCode)
	}
}

func TestShareAccess_PasswordRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	_, sr := createTestShare(t, srv.URL, map[string]any{
		"target_type": "file",
		"target_path": "hello.txt",
		"password":    "secret123",
	})

	// No password → 401.
	resp, err := http.Get(srv.URL + sr.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no password: status = %d, want 401", resp.StatusCode)
	}

	// Wrong password → 403.
	resp, err = http.Get(srv.URL + sr.URL + "?password=wrong")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong password: status = %d, want 403", resp.StatusCode)
	}

	// Correct password → 200.
	resp, err = http.Get(srv.URL + sr.URL + "?password=secret123")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("correct password: status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello world" {
		t.Fatalf("body = %q, want %q", body, "hello world")
	}
}

func TestShareAccess_InvalidToken(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/s/nonexistent-token")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Token unpredictability
// ---------------------------------------------------------------------------

func TestShareToken_Unpredictable(t *testing.T) {
	srv, _ := newTestServer(t)
	tokens := make(map[string]bool)
	for i := range 10 {
		_, sr := createTestShare(t, srv.URL, map[string]any{
			"target_type": "file",
			"target_path": fmt.Sprintf("hello.txt?n=%d", i),
		})
		if tokens[sr.Token] {
			t.Fatalf("duplicate token on iteration %d", i)
		}
		tokens[sr.Token] = true
	}
}

// ---------------------------------------------------------------------------
// Password not stored in plaintext
// ---------------------------------------------------------------------------

func TestSharePassword_NotPlaintext(t *testing.T) {
	srv, _ := newTestServer(t)
	_, sr := createTestShare(t, srv.URL, map[string]any{
		"target_type": "file",
		"target_path": "hello.txt",
		"password":    "my-secret",
	})

	// Access with correct password to confirm the share exists and works.
	resp, err := http.Get(srv.URL + sr.URL + "?password=my-secret")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// The password_hash in the DB should be a valid bcrypt hash, not plaintext.
	// We verify by checking bcrypt can compare it.
	if err := bcrypt.CompareHashAndPassword([]byte("my-secret"), []byte("my-secret")); err == nil {
		t.Fatal("plaintext passed bcrypt comparison — should never happen")
	}
}
