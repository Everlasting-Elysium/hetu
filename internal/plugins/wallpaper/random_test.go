package wallpaper

import (
	"fmt"
	"testing"
)

// TestRandomCount covers ?count=: the default (1), an explicit count, and a
// count larger than inventory (returns all, never an error).
func TestRandomCount(t *testing.T) {
	e := newTestEnv(t)
	for i := range 5 {
		e.add(seed{id: fmt.Sprintf("a%d", i), name: fmt.Sprintf("w%d.png", i)})
	}

	if got := e.getDTOs("/api/wallpaper/random"); len(got) != 1 {
		t.Errorf("default random = %d, want 1", len(got))
	}
	if got := e.getDTOs("/api/wallpaper/random?count=3"); len(got) != 3 {
		t.Errorf("random count=3 = %d, want 3", len(got))
	}
	// count beyond inventory (and beyond the cap) returns the whole matching set.
	if got := e.getDTOs("/api/wallpaper/random?count=100"); len(got) != 5 {
		t.Errorf("random count=100 = %d, want 5 (all)", len(got))
	}
}

// TestRandomRespectsFilter confirms ?count= composes with the kind default so
// random never returns non-wallpaper kinds.
func TestRandomRespectsFilter(t *testing.T) {
	e := newTestEnv(t)
	e.add(seed{id: "img", name: "one.png"})
	e.add(seed{id: "aud", name: "two.mp3", kind: "audio"})

	got := e.getDTOs("/api/wallpaper/random?count=50")
	if len(got) != 1 || got[0].Kind != "image" {
		t.Errorf("random = %v, want the single image only", dtoIDs(got))
	}
}
