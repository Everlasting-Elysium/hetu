package dam_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestBoardItemInvalidCombos verifies the addBoardItem handler rejects
// kind/asset_id/text combinations that violate domain.ValidateBoardItem.
func TestBoardItemInvalidCombos(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	seedTestAsset(t, ctx, st, owner, "a1", "photo.png")

	body, _ := json.Marshal(map[string]string{"name": "Board"})
	resp, _ := http.Post(srv.URL+"/api/dam/boards", "application/json", bytes.NewReader(body))
	var board boardResult
	json.NewDecoder(resp.Body).Decode(&board)
	resp.Body.Close()
	itemsURL := srv.URL + "/api/dam/boards/" + board.ID + "/items"

	cases := []struct {
		name string
		body map[string]any
	}{
		{"note with asset_id", map[string]any{"kind": "note", "text": "hi", "asset_id": "a1"}},
		{"asset without asset_id", map[string]any{"kind": "asset"}},
		{"note with blank text", map[string]any{"kind": "note", "text": "   "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(tc.body)
			r, err := http.Post(itemsURL, "application/json", bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			r.Body.Close()
			if r.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", r.StatusCode)
			}
		})
	}
}
