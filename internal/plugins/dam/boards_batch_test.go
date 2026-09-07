package dam_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// postBatchToBoard sends {"asset_ids": ids} to a board's /items/batch endpoint.
func postBatchToBoard(t *testing.T, boardURL string, ids []string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string][]string{"asset_ids": ids})
	resp, err := http.Post(boardURL+"/items/batch", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// createTestBoard creates an empty board and returns it.
func createTestBoard(t *testing.T, srvURL string) boardResult {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": "Board"})
	resp, err := http.Post(srvURL+"/api/dam/boards", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var board boardResult
	if err := json.NewDecoder(resp.Body).Decode(&board); err != nil {
		t.Fatal(err)
	}
	return board
}

// getBoardDetail fetches a board with its items.
func getBoardDetail(t *testing.T, boardURL string) boardDetailResult {
	t.Helper()
	resp, err := http.Get(boardURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var detail boardDetailResult
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	return detail
}

// decodeAdded reads the {"added": n} batch response.
func decodeAdded(t *testing.T, resp *http.Response) int {
	t.Helper()
	defer resp.Body.Close()
	var out struct {
		Added int `json:"added"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Added
}

func TestBatchAddToBoard(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	seedTestAsset(t, ctx, st, owner, "a1", "one.png")
	seedTestAsset(t, ctx, st, owner, "a2", "two.png")
	seedTestAsset(t, ctx, st, owner, "a3", "three.png")

	board := createTestBoard(t, srv.URL)
	boardURL := srv.URL + "/api/dam/boards/" + board.ID

	resp := postBatchToBoard(t, boardURL, []string{"a1", "a2", "a3"})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if n := decodeAdded(t, resp); n != 3 {
		t.Fatalf("added = %d, want 3", n)
	}

	// All three land at the schema default size and distinct positions, so a
	// batch sent from the list never stacks at (0,0).
	detail := getBoardDetail(t, boardURL)
	if len(detail.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(detail.Items))
	}
	seen := map[string]bool{}
	for _, it := range detail.Items {
		if it.W != 200 || it.H != 200 {
			t.Fatalf("item size = (%v,%v), want (200,200)", it.W, it.H)
		}
		pos := fmt.Sprintf("%v,%v", it.X, it.Y)
		if seen[pos] {
			t.Fatalf("items overlap at %s", pos)
		}
		seen[pos] = true
	}
}

func TestBatchAddToBoardSkipsDuplicates(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	seedTestAsset(t, ctx, st, owner, "a1", "one.png")
	seedTestAsset(t, ctx, st, owner, "a2", "two.png")

	board := createTestBoard(t, srv.URL)
	boardURL := srv.URL + "/api/dam/boards/" + board.ID

	// Seed a1 onto the board.
	postBatchToBoard(t, boardURL, []string{"a1"}).Body.Close()

	// Re-send a1 (already present) + a2 (new) + a2 (repeated in-request): only
	// the one new asset is written.
	resp := postBatchToBoard(t, boardURL, []string{"a1", "a2", "a2"})
	if n := decodeAdded(t, resp); n != 1 {
		t.Fatalf("added = %d, want 1 (a1 already present, a2 collapsed)", n)
	}
	detail := getBoardDetail(t, boardURL)
	if len(detail.Items) != 2 {
		t.Fatalf("items = %d, want 2 (a1, a2)", len(detail.Items))
	}
}

func TestBatchAddToBoardValidation(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	seedTestAsset(t, ctx, st, owner, "a1", "one.png")

	board := createTestBoard(t, srv.URL)
	boardURL := srv.URL + "/api/dam/boards/" + board.ID

	// Empty asset_ids -> 400.
	resp := postBatchToBoard(t, boardURL, []string{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty ids status = %d, want 400", resp.StatusCode)
	}

	// Non-existent (or non-owned) board -> 404.
	resp = postBatchToBoard(t, srv.URL+"/api/dam/boards/does-not-exist", []string{"a1"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("ghost board status = %d, want 404", resp.StatusCode)
	}

	// Non-existent asset -> 404 and nothing is written (all-or-nothing).
	resp = postBatchToBoard(t, boardURL, []string{"a1", "ghost"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("ghost asset status = %d, want 404", resp.StatusCode)
	}
	if detail := getBoardDetail(t, boardURL); len(detail.Items) != 0 {
		t.Fatalf("items after rejected add = %d, want 0", len(detail.Items))
	}
}

// TestBatchAddPreservesAspect guards the fix for the squashing bug: a batch send
// must scale each asset to its own aspect ratio, not a fixed 200x200 square.
func TestBatchAddPreservesAspect(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	// A landscape 400x200 and a portrait 200x400 — clearly different aspects.
	seedSizedAsset(t, ctx, st, owner, "wide", "wide.png", 400, 200)
	seedSizedAsset(t, ctx, st, owner, "tall", "tall.png", 200, 400)

	board := createTestBoard(t, srv.URL)
	boardURL := srv.URL + "/api/dam/boards/" + board.ID
	postBatchToBoard(t, boardURL, []string{"wide", "tall"}).Body.Close()

	items := map[string]boardItemResult{}
	for _, it := range getBoardDetail(t, boardURL).Items {
		items[it.AssetID] = it
	}
	// 400x200: longest 400 -> scale 0.5 -> 200x100, aspect 2:1 preserved.
	if w := items["wide"]; w.W != 200 || w.H != 100 {
		t.Fatalf("wide size = (%v,%v), want (200,100) — aspect must survive batch add", w.W, w.H)
	}
	// 200x400: longest 400 -> scale 0.5 -> 100x200, aspect 1:2 preserved.
	if tall := items["tall"]; tall.W != 100 || tall.H != 200 {
		t.Fatalf("tall size = (%v,%v), want (100,200) — aspect must survive batch add", tall.W, tall.H)
	}
}
