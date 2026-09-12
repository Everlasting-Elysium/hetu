package imgcompare

import "image"

// Tone scores brightness-histogram similarity between the reference and target
// images. Standalone entry point; when both tone and lighting are needed, prefer
// ToneAndLighting so the per-image luma pass runs only once.
func Tone(ref, target image.Image) ToneResult {
	return toneResultFromFields(newLumaField(ref), newLumaField(target))
}

// Lighting scores shadow/highlight-zone similarity between the reference and
// target images. Standalone entry point; prefer ToneAndLighting when tone is
// also needed.
func Lighting(ref, target image.Image) LightingResult {
	return lightingResultFromFields(newLumaField(ref), newLumaField(target))
}

// ToneAndLighting computes both luma-driven dimensions from a single per-image
// luma pass: each image's field is built once and shared, so the lighting zones
// reuse exactly the per-pixel luma array the tone histogram was built from. This
// is the path the HTTP compare endpoint uses when it reports both dimensions.
func ToneAndLighting(ref, target image.Image) (ToneResult, LightingResult) {
	refField, tgtField := newLumaField(ref), newLumaField(target)
	return toneResultFromFields(refField, tgtField), lightingResultFromFields(refField, tgtField)
}
