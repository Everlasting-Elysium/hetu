// Package vecmath provides vector math utilities for CLIP embedding search.
// Vectors are L2-normalized float32 slices; cosine similarity reduces to a dot
// product. Serialization is little-endian float32 for SQLite BLOB storage.
package vecmath

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Dot returns the dot product of two equal-length float32 vectors. For
// L2-normalized vectors (as CLIP outputs are) this equals cosine similarity,
// with result in [-1, 1] where 1 means identical direction. Panics if the
// vectors differ in length.
func Dot(a, b []float32) float64 {
	if len(a) != len(b) {
		panic(fmt.Sprintf("vecmath.Dot: length mismatch: %d vs %d", len(a), len(b)))
	}
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return float64(dot)
}

// Float32sToBytes serializes a float32 slice to a little-endian byte slice
// suitable for SQLite BLOB storage.
func Float32sToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// BytesToFloat32s deserializes a little-endian byte slice back to float32s.
// Returns an error if the byte length is not a multiple of 4.
func BytesToFloat32s(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("vecmath: byte length %d is not a multiple of 4", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v, nil
}
