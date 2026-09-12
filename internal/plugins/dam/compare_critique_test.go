package dam_test

import (
	"context"
	"encoding/json"
	"image/color"
	"io"
	"net/http"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/ai"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// critiqueResp decodes the full critique object (compareResp in compare_test.go
// only observes Available; the summary/dimensions/model fields need their own
// mirror to assert the success path populates them).
type critiqueResp struct {
	ReqID    string `json:"req_id"`
	Critique struct {
		Available  bool              `json:"available"`
		Summary    string            `json:"summary"`
		Dimensions map[string]string `json:"dimensions"`
		Model      string            `json:"model"`
	} `json:"critique"`
}

// fakeVisionCritic is a kernel.VisionCritic stub that records what it was asked
// and returns a canned result or error.
type fakeVisionCritic struct {
	result  kernel.CritiqueResult
	err     error
	called  bool
	gotDims []string
}

func (f *fakeVisionCritic) Critique(_ context.Context, _, _ string, dims []string) (kernel.CritiqueResult, error) {
	f.called = true
	f.gotDims = dims
	return f.result, f.err
}

func decodeCritique(t *testing.T, resp *http.Response) critiqueResp {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, b)
	}
	var out critiqueResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func seedCritiquePair(t *testing.T, st kernel.Store, owner domain.OwnerID, lib string) {
	t.Helper()
	seedCompareAsset(t, st, owner, lib, "ref1", "ref.png", color.RGBA{R: 200, G: 80, B: 40, A: 255})
	seedCompareAsset(t, st, owner, lib, "tgt1", "tgt.png", color.RGBA{R: 40, G: 90, B: 210, A: 255})
}

// TestCompare_CritiqueUnavailableWhenNoCritic is the regression guard: with a
// nil VisionCritic (the default), the PR2 degradation contract must hold —
// critique.available is false and the request is still 200.
func TestCompare_CritiqueUnavailableWhenNoCritic(t *testing.T) {
	srv, owner, st, lib, k := compareFixtureWithKernel(t)
	if k.VisionCritic != nil {
		t.Fatal("fixture must default to a nil VisionCritic")
	}
	seedCritiquePair(t, st, owner, lib)

	got := decodeCritique(t, postCompare(t, srv.URL,
		comparePart{field: "reference_asset_id", value: "ref1"},
		comparePart{field: "target_asset_id", value: "tgt1"},
	))
	if got.Critique.Available {
		t.Fatal("critique.available must be false when no VisionCritic is configured")
	}
}

// TestCompare_CritiquePopulatedOnSuccess proves the wiring: a VisionCritic that
// returns a result fills the response, and it is asked about the dimensions
// actually computed (color/tone/lighting; action/element are omitted with no
// Tagger), not merely the requested set.
func TestCompare_CritiquePopulatedOnSuccess(t *testing.T) {
	srv, owner, st, lib, k := compareFixtureWithKernel(t)
	critic := &fakeVisionCritic{result: kernel.CritiqueResult{
		Summary:    "target is cooler and darker",
		Dimensions: map[string]string{"color": "warm it up", "tone": "lift midtones"},
		Model:      "SmolVLM-500M-Instruct",
	}}
	k.VisionCritic = critic
	seedCritiquePair(t, st, owner, lib)

	got := decodeCritique(t, postCompare(t, srv.URL,
		comparePart{field: "reference_asset_id", value: "ref1"},
		comparePart{field: "target_asset_id", value: "tgt1"},
	))
	if !got.Critique.Available {
		t.Fatal("critique.available must be true when the VisionCritic returns a result")
	}
	if got.Critique.Summary != "target is cooler and darker" {
		t.Errorf("summary = %q", got.Critique.Summary)
	}
	if got.Critique.Model != "SmolVLM-500M-Instruct" {
		t.Errorf("model = %q", got.Critique.Model)
	}
	if got.Critique.Dimensions["color"] != "warm it up" {
		t.Errorf("dimensions = %v", got.Critique.Dimensions)
	}
	if !critic.called {
		t.Fatal("VisionCritic.Critique was not called")
	}
	want := map[string]bool{"color": true, "tone": true, "lighting": true}
	if len(critic.gotDims) != len(want) {
		t.Fatalf("critique dims = %v, want color/tone/lighting", critic.gotDims)
	}
	for _, d := range critic.gotDims {
		if !want[d] {
			t.Errorf("unexpected critique dim %q (must be a computed dimension)", d)
		}
	}
}

// TestCompare_CritiqueDegradesOnError proves a failing critic (here a 501 /
// ai.ErrNotImplemented, standing in for any error) never fails /compare: the
// response is 200 with critique.available false.
func TestCompare_CritiqueDegradesOnError(t *testing.T) {
	srv, owner, st, lib, k := compareFixtureWithKernel(t)
	k.VisionCritic = &fakeVisionCritic{err: ai.ErrNotImplemented}
	seedCritiquePair(t, st, owner, lib)

	got := decodeCritique(t, postCompare(t, srv.URL,
		comparePart{field: "reference_asset_id", value: "ref1"},
		comparePart{field: "target_asset_id", value: "tgt1"},
	))
	if got.Critique.Available {
		t.Fatal("critique.available must be false when the VisionCritic errors")
	}
}
