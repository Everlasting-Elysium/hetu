package ai

import (
	"context"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// embedAdapter wraps *Client to implement kernel.Embedder.
type embedAdapter struct {
	client *Client
}

// NewEmbedder returns a kernel.Embedder backed by the AI sidecar client.
func NewEmbedder(client *Client) kernel.Embedder {
	return &embedAdapter{client: client}
}

func (a *embedAdapter) Embed(ctx context.Context, ref string) ([]float32, error) {
	res, err := a.client.Embed(ctx, AssetRef{Ref: ref})
	if err != nil {
		return nil, err
	}
	return res.Vector, nil
}
