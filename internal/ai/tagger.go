package ai

import (
	"context"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// tagAdapter wraps *Client to implement kernel.Tagger.
type tagAdapter struct {
	client *Client
}

// NewTagger returns a kernel.Tagger backed by the AI sidecar client. It exposes
// the sidecar's /tag capability synchronously (name -> confidence), so callers
// can score a just-uploaded image that never went through the async scan-time
// tagging pipeline (see SubscribeEmbedding's counterpart, Subscribe).
func NewTagger(client *Client) kernel.Tagger {
	return &tagAdapter{client: client}
}

func (a *tagAdapter) Tag(ctx context.Context, ref string) (map[string]float64, error) {
	res, err := a.client.Tag(ctx, AssetRef{Ref: ref})
	if err != nil {
		return nil, err
	}
	tags := make(map[string]float64, len(res.Tags))
	for _, t := range res.Tags {
		tags[t.Name] = t.Confidence
	}
	return tags, nil
}
