package vecmath

import (
	"math"
	"testing"
)

func TestFloat32sRoundTrip(t *testing.T) {
	orig := []float32{0.1, -0.5, 3.14, 0, math.MaxFloat32, math.SmallestNonzeroFloat32}
	b := Float32sToBytes(orig)
	got, err := BytesToFloat32s(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(orig) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(orig))
	}
	for i := range orig {
		if got[i] != orig[i] {
			t.Errorf("[%d] got %v, want %v", i, got[i], orig[i])
		}
	}
}

func TestBytesToFloat32sInvalidLength(t *testing.T) {
	_, err := BytesToFloat32s([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for non-multiple-of-4 length")
	}
}

func TestBytesToFloat32sEmpty(t *testing.T) {
	got, err := BytesToFloat32s(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d elements", len(got))
	}
}

func TestDotIdentical(t *testing.T) {
	// L2-normalized vector: dot with itself should be 1.
	v := normalize([]float32{1, 2, 3, 4})
	sim := Dot(v, v)
	if math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("self-similarity = %v, want 1.0", sim)
	}
}

func TestDotOrthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	sim := Dot(a, b)
	if math.Abs(sim) > 1e-6 {
		t.Errorf("orthogonal similarity = %v, want 0.0", sim)
	}
}

func TestDotOpposite(t *testing.T) {
	a := normalize([]float32{1, 2, 3})
	b := make([]float32, len(a))
	for i := range a {
		b[i] = -a[i]
	}
	sim := Dot(a, b)
	if math.Abs(sim+1.0) > 1e-6 {
		t.Errorf("opposite similarity = %v, want -1.0", sim)
	}
}

func TestDotLengthMismatchPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for mismatched lengths")
		}
	}()
	Dot([]float32{1, 2}, []float32{1})
}

func normalize(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}
