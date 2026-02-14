package evaluator

import "context"

// Embedder produces a vector embedding for text (e.g. for similarity comparison).
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
