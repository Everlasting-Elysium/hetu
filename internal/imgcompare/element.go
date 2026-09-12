package imgcompare

import "math"

// elementTagWeight and elementEmbedWeight blend the tag-overlap and embedding-
// similarity halves of the element score. Equal placeholder weights, tunable by
// a later PR once real usage exists.
const (
	elementTagWeight   = 0.5
	elementEmbedWeight = 0.5
)

// ElementInput is one image's element-dimension features: the full WD-tagger tag
// set (name->confidence) and its CLIP embedding. Bundled so Element takes one
// value per image instead of four loose parameters.
type ElementInput struct {
	Tags      map[string]float64
	Embedding []float32
}

// ElementResult is the subject/element comparison from the full tag sets plus
// embedding cosine similarity.
type ElementResult struct {
	Score      float64  `json:"score"`
	Common     []string `json:"common"`      // tags in both
	Missing    []string `json:"missing"`     // tags in reference only
	Extra      []string `json:"extra"`       // tags in target only
	TagJaccard float64  `json:"tag_jaccard"` // |common|/|union| in [0,1]
	Cosine     float64  `json:"cosine"`      // embedding cosine in [-1,1]
}

// Element compares subjects/elements across the full tag sets and the CLIP
// embeddings. Tag Jaccard and embedding cosine each contribute half the score;
// cosine [-1,1] is remapped to [0,1] as (cos+1)/2. When either embedding is
// absent or length-mismatched the score falls back to tag Jaccard alone, so a
// tags-only caller still gets a meaningful number. Pure function, no I/O.
func Element(ref, target ElementInput) ElementResult {
	common, missing, extra := diffKeys(ref.Tags, target.Tags)
	tagJ := jaccard(common, missing, extra)
	cos := cosine(ref.Embedding, target.Embedding)

	score := 100 * tagJ
	if usableEmbedding(ref.Embedding, target.Embedding) {
		cosNorm := (cos + 1) / 2
		score = 100 * clamp01(elementTagWeight*tagJ+elementEmbedWeight*cosNorm)
	}
	return ElementResult{
		Score:      score,
		Common:     common,
		Missing:    missing,
		Extra:      extra,
		TagJaccard: tagJ,
		Cosine:     cos,
	}
}

// usableEmbedding reports whether both embeddings are present and equal length,
// the precondition for cosine to be meaningful.
func usableEmbedding(a, b []float32) bool {
	return len(a) > 0 && len(a) == len(b)
}

// cosine is the cosine similarity of two equal-length vectors: dot(a,b) /
// (|a|*|b|), in [-1,1]. Returns 0 for empty or mismatched vectors. Implemented
// locally (a handful of lines) to keep imgcompare free of any internal package
// beyond color; internal/vecmath serves the kernel store's vector search, not
// this pure metric library.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
