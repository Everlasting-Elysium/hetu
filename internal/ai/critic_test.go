package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

func TestContractVersionIsV2(t *testing.T) {
	// The Go and Python sides carry this string independently (ai/schemas.py
	// CONTRACT_VERSION); each asserts its own value so a one-sided bump is caught.
	if ContractVersion != "v2" {
		t.Errorf("ContractVersion = %q, want %q", ContractVersion, "v2")
	}
}

func TestClient_Compare(t *testing.T) {
	c := testClient(newFakeSidecar(t).URL)
	res, err := c.Compare(context.Background(), CompareRequest{
		RefA: "a.png", RefB: "b.png", Dimensions: []string{"color", "tone"},
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.Summary != "target is cooler and darker" {
		t.Errorf("summary = %q", res.Summary)
	}
	if res.Model != "stub-vlm" {
		t.Errorf("model = %q", res.Model)
	}
	if res.Dimensions["color"] != "warm it up" || res.Dimensions["tone"] != "lift midtones" {
		t.Errorf("dimensions = %v", res.Dimensions)
	}
}

// TestNewVisionCritic exercises the kernel.VisionCritic adapter end-to-end: the
// sidecar's CompareResult must map field-for-field onto kernel.CritiqueResult.
func TestNewVisionCritic(t *testing.T) {
	critic := NewVisionCritic(testClient(newFakeSidecar(t).URL))
	res, err := critic.Critique(context.Background(), "a.png", "b.png", []string{"color", "tone"})
	if err != nil {
		t.Fatalf("Critique: %v", err)
	}
	want := kernel.CritiqueResult{
		Summary:    "target is cooler and darker",
		Dimensions: map[string]string{"color": "warm it up", "tone": "lift midtones"},
		Model:      "stub-vlm",
	}
	if res.Summary != want.Summary || res.Model != want.Model {
		t.Errorf("critique = %+v, want %+v", res, want)
	}
	if res.Dimensions["color"] != want.Dimensions["color"] || res.Dimensions["tone"] != want.Dimensions["tone"] {
		t.Errorf("dimensions = %v, want %v", res.Dimensions, want.Dimensions)
	}
}

// TestNewVisionCritic_NotImplementedPropagates proves the 501 path survives the
// whole chain: sidecar 501 -> client ErrNotImplemented -> adapter propagates,
// which is what lets the DAM compare handler degrade to an unavailable critique.
func TestNewVisionCritic_NotImplementedPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"detail": "HETU_AI_VLM_MODEL not configured"})
	}))
	t.Cleanup(srv.Close)

	critic := NewVisionCritic(testClient(srv.URL))
	_, err := critic.Critique(context.Background(), "a.png", "b.png", []string{"color"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}
