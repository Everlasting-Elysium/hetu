package ai

import (
	"context"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// criticAdapter wraps *Client to implement kernel.VisionCritic.
type criticAdapter struct {
	client *Client
}

// NewVisionCritic returns a kernel.VisionCritic backed by the AI sidecar client.
// It exposes the sidecar's /compare capability (a VLM's natural-language
// comparison of two images); a sidecar without a VLM configured returns 501,
// which the client surfaces as ErrNotImplemented so callers degrade gracefully.
func NewVisionCritic(client *Client) kernel.VisionCritic {
	return &criticAdapter{client: client}
}

func (a *criticAdapter) Critique(ctx context.Context, refA, refB string, dimensions []string) (kernel.CritiqueResult, error) {
	res, err := a.client.Compare(ctx, CompareRequest{RefA: refA, RefB: refB, Dimensions: dimensions})
	if err != nil {
		return kernel.CritiqueResult{}, err
	}
	return kernel.CritiqueResult{Summary: res.Summary, Dimensions: res.Dimensions, Model: res.Model}, nil
}
