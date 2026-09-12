package imgcompare_test

import (
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/imgcompare"
)

// TestElementCosine exercises the local cosine similarity through the public
// ElementResult.Cosine field with known vectors: orthogonal -> 0, same-direction
// -> 1, opposite -> -1, plus the empty/mismatched guards -> 0.
func TestElementCosine(t *testing.T) {
	cases := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 0},
		{"parallel", []float32{1, 2, 3}, []float32{2, 4, 6}, 1},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1},
		{"identical", []float32{0.3, 0.4}, []float32{0.3, 0.4}, 1},
		{"mismatched-len", []float32{1, 0}, []float32{1, 0, 0}, 0},
		{"empty", []float32{}, []float32{}, 0},
	}
	for _, c := range cases {
		got := imgcompare.Element(
			imgcompare.ElementInput{Embedding: c.a},
			imgcompare.ElementInput{Embedding: c.b},
		).Cosine
		if !approx(got, c.want, 1e-9) {
			t.Fatalf("cosine(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestElementTagsOnly: with no embeddings the score is pure tag Jaccard. cat/sky
// are common, tree missing, dog extra -> Jaccard 2/4 = 0.5 -> score 50.
func TestElementTagsOnly(t *testing.T) {
	ref := imgcompare.ElementInput{Tags: map[string]float64{"cat": 0.9, "tree": 0.5, "sky": 0.7}}
	tgt := imgcompare.ElementInput{Tags: map[string]float64{"cat": 0.8, "dog": 0.6, "sky": 0.4}}

	res := imgcompare.Element(ref, tgt)

	if !sameStrings(res.Common, []string{"cat", "sky"}) {
		t.Fatalf("common = %v, want [cat sky]", res.Common)
	}
	if !sameStrings(res.Missing, []string{"tree"}) || !sameStrings(res.Extra, []string{"dog"}) {
		t.Fatalf("missing/extra = %v / %v, want [tree] / [dog]", res.Missing, res.Extra)
	}
	if !approx(res.TagJaccard, 0.5, 1e-9) || !approx(res.Score, 50, 1e-9) {
		t.Fatalf("jaccard=%v score=%v, want 0.5 / 50", res.TagJaccard, res.Score)
	}
	if !approx(res.Cosine, 0, 1e-9) {
		t.Fatalf("cosine = %v, want 0 (no embeddings)", res.Cosine)
	}
}

// TestElementBlend: identical tags + identical embedding -> 100; identical tags +
// orthogonal embedding -> 0.5*1 + 0.5*0.5 = 0.75 -> 75.
func TestElementBlend(t *testing.T) {
	same := imgcompare.ElementInput{Tags: map[string]float64{"cat": 0.9}, Embedding: []float32{1, 1}}
	if res := imgcompare.Element(same, same); !approx(res.Score, 100, 1e-9) {
		t.Fatalf("identical element score = %v, want 100", res.Score)
	}

	ref := imgcompare.ElementInput{Tags: map[string]float64{"cat": 0.9}, Embedding: []float32{1, 0}}
	tgt := imgcompare.ElementInput{Tags: map[string]float64{"cat": 0.8}, Embedding: []float32{0, 1}}
	res := imgcompare.Element(ref, tgt)
	if !approx(res.TagJaccard, 1, 1e-9) || !approx(res.Cosine, 0, 1e-9) {
		t.Fatalf("jaccard=%v cosine=%v, want 1 / 0", res.TagJaccard, res.Cosine)
	}
	if !approx(res.Score, 75, 1e-9) {
		t.Fatalf("blend score = %v, want 75", res.Score)
	}
}
