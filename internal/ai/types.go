// Package ai is the Go client and job orchestration for hetu's Python AI
// sidecar (see ai/server.py). The kernel never loads ML models; it talks to the
// sidecar over a small, versioned HTTP contract and reacts to indexing events by
// enqueueing tagging jobs on kernel.JobQueue.
//
// # Contract (version [ContractVersion])
//
// Every request carries the header [HeaderContractVersion] so the sidecar can
// reject an incompatible core. The transport is JSON over HTTP; all POST bodies
// are an [AssetRef] the sidecar resolves locally (local-first, per
// docs/ai-and-3d.md).
//
//	GET  /health -> 200 {"ok": true}                       [Health]
//	POST /embed  -> 200 {"vector":[...],"dim":N,"model":""} [EmbedResult]
//	POST /tag    -> 200 {"tags":[...],"caption":"","model":""} [TagResult]
//	POST /caption-> 200 {"caption":"","model":""}           [CaptionResult]
//	POST /ocr    -> 200 {"text":"","blocks":[...],"model":""} [OCRResult]
//
// The Phase 1 sidecar is a stub: /embed and /tag return 501 Not Implemented,
// which the client maps to [ErrNotImplemented] (kind [KindInvalid], not
// retried). Real models land in a separate issue (#11); this package is the
// stable seam they plug into.
//
// # Errors
//
// Every method returns a typed [*Error] carrying a [Kind]. Only network,
// timeout, and rate_limit kinds are retried (see [RetryPolicy]); auth, quota,
// and invalid are terminal. Callers classify with errors.As(err, &*Error) or
// errors.Is against [ErrNotImplemented].
package ai

// ContractVersion is the AI HTTP contract version. Bump it on any
// breaking change to a request or response shape.
const ContractVersion = "v1"

// HeaderContractVersion is the request header carrying [ContractVersion].
const HeaderContractVersion = "X-Hetu-AI-Contract"

// AssetRef is the body of every inference request: a storage path or URL the
// sidecar resolves to the asset bytes. Phase 1 fixes the transport shape.
type AssetRef struct {
	Ref string `json:"ref"`
}

// Health is the /health response. OK is true when the sidecar is ready.
type Health struct {
	OK bool `json:"ok"`
}

// Tag is a single predicted label with its confidence in [0,1].
type Tag struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

// TagResult is the /tag response: labels plus an optional caption. Model names
// the producing model so results can be re-run when a model is upgraded.
type TagResult struct {
	Tags    []Tag  `json:"tags"`
	Caption string `json:"caption"`
	Model   string `json:"model"`
}

// EmbedResult is the /embed response: a CLIP embedding vector and its length.
type EmbedResult struct {
	Vector []float32 `json:"vector"`
	Dim    int       `json:"dim"`
	Model  string    `json:"model"`
}

// CaptionResult is the /caption response: a natural-language description.
type CaptionResult struct {
	Caption string `json:"caption"`
	Model   string `json:"model"`
}

// OCRBlock is one detected text run with its confidence and bounding box
// [x0,y0,x1,y1] in pixels.
type OCRBlock struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	BBox       [4]int  `json:"bbox"`
}

// OCRResult is the /ocr response: the full extracted text plus per-block detail.
type OCRResult struct {
	Text   string     `json:"text"`
	Blocks []OCRBlock `json:"blocks,omitempty"`
	Model  string     `json:"model"`
}
