package dam_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// seedTestAsset upserts a minimal image asset for board item tests.
func seedTestAsset(t *testing.T, ctx context.Context, st kernel.Store, owner domain.OwnerID, id, path string) {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: path, Name: path, Ext: "png", Size: 1, Hash: "h-" + id,
		Width: 1, Height: 1, CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("seed asset %s: %v", id, err)
	}
}

type boardResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type boardDetailResult struct {
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Items []boardItemResult `json:"items"`
}

type boardItemResult struct {
	ID       string  `json:"id"`
	AssetID  string  `json:"asset_id"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
	Rotation float64 `json:"rotation"`
	Z        int     `json:"z"`
}

func TestBoardCRUD(t *testing.T) {
	srv, _, _ := newTestServer(t)

	// Create board.
	body, _ := json.Marshal(map[string]string{"name": "Moodboard"})
	resp, err := http.Post(srv.URL+"/api/dam/boards", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created boardResult
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "Moodboard" || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}

	// List boards.
	resp2, err := http.Get(srv.URL + "/api/dam/boards")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var boards []boardResult
	if err := json.NewDecoder(resp2.Body).Decode(&boards); err != nil {
		t.Fatal(err)
	}
	if len(boards) != 1 || boards[0].Name != "Moodboard" {
		t.Fatalf("list = %+v, want [Moodboard]", boards)
	}

	// Rename board.
	renameBody, _ := json.Marshal(map[string]string{"name": "Renamed"})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/dam/boards/"+created.ID, bytes.NewReader(renameBody))
	req.Header.Set("Content-Type", "application/json")
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d, want 200", resp3.StatusCode)
	}

	// Get board (should have new name and empty items).
	resp4, err := http.Get(srv.URL + "/api/dam/boards/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	var detail boardDetailResult
	if err := json.NewDecoder(resp4.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Name != "Renamed" {
		t.Fatalf("name = %q, want Renamed", detail.Name)
	}
	if len(detail.Items) != 0 {
		t.Fatalf("items = %d, want 0", len(detail.Items))
	}

	// Delete board.
	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/dam/boards/"+created.ID, nil)
	resp5, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", resp5.StatusCode)
	}

	// List should be empty.
	resp6, err := http.Get(srv.URL + "/api/dam/boards")
	if err != nil {
		t.Fatal(err)
	}
	defer resp6.Body.Close()
	var empty []boardResult
	json.NewDecoder(resp6.Body).Decode(&empty)
	if len(empty) != 0 {
		t.Fatalf("after delete = %d, want 0", len(empty))
	}
}

func TestBoardItems(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()

	// Seed an asset.
	seedTestAsset(t, ctx, st, owner, "a1", "photo.png")

	// Create a board.
	body, _ := json.Marshal(map[string]string{"name": "Board"})
	resp, _ := http.Post(srv.URL+"/api/dam/boards", "application/json", bytes.NewReader(body))
	var board boardResult
	json.NewDecoder(resp.Body).Decode(&board)
	resp.Body.Close()

	boardURL := srv.URL + "/api/dam/boards/" + board.ID

	// Add item.
	itemBody, _ := json.Marshal(map[string]any{
		"asset_id": "a1", "x": 10.0, "y": 20.0,
		"w": 300.0, "h": 200.0, "rotation": 45.0, "z": 0,
	})
	resp2, err := http.Post(boardURL+"/items", "application/json", bytes.NewReader(itemBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("add item status = %d, want 201", resp2.StatusCode)
	}
	var item boardItemResult
	json.NewDecoder(resp2.Body).Decode(&item)
	if item.X != 10 || item.Y != 20 || item.Rotation != 45 {
		t.Fatalf("item = %+v", item)
	}

	// Get board with items.
	resp3, _ := http.Get(boardURL)
	var detail boardDetailResult
	json.NewDecoder(resp3.Body).Decode(&detail)
	resp3.Body.Close()
	if len(detail.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(detail.Items))
	}

	// Batch update items.
	detail.Items[0].X = 50
	detail.Items[0].Y = 60
	detail.Items[0].Z = 3
	updateBody, _ := json.Marshal(map[string]any{"items": detail.Items})
	patchReq, _ := http.NewRequest(http.MethodPatch, boardURL+"/items", bytes.NewReader(updateBody))
	patchReq.Header.Set("Content-Type", "application/json")
	resp4, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatal(err)
	}
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("batch update status = %d, want 200", resp4.StatusCode)
	}

	// Verify persistence.
	resp5, _ := http.Get(boardURL)
	var updated boardDetailResult
	json.NewDecoder(resp5.Body).Decode(&updated)
	resp5.Body.Close()
	if len(updated.Items) != 1 || updated.Items[0].X != 50 || updated.Items[0].Y != 60 {
		t.Fatalf("after update = %+v", updated.Items)
	}

	// Delete item.
	delReq, _ := http.NewRequest(http.MethodDelete, boardURL+"/items/"+item.ID, nil)
	resp6, _ := http.DefaultClient.Do(delReq)
	resp6.Body.Close()
	if resp6.StatusCode != http.StatusOK {
		t.Fatalf("delete item status = %d, want 200", resp6.StatusCode)
	}

	resp7, _ := http.Get(boardURL)
	var afterDel boardDetailResult
	json.NewDecoder(resp7.Body).Decode(&afterDel)
	resp7.Body.Close()
	if len(afterDel.Items) != 0 {
		t.Fatalf("after item delete = %d, want 0", len(afterDel.Items))
	}
}
