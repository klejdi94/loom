package evaluator

import (
	"context"
	"math"
)

// Similarity is an evaluator that scores actual vs expected using embedding cosine similarity.
type Similarity struct {
	Embedder Embedder
	// Threshold is the minimum cosine similarity (0-1) to pass. Default 0.85.
	Threshold float64
}

// Evaluate implements Evaluator.
func (s *Similarity) Evaluate(ctx context.Context, actual string, expected Expected) (Score, error) {
	threshold := s.Threshold
	if threshold <= 0 {
		threshold = 0.85
	}
	if s.Embedder == nil {
		return Score{Pass: false, Value: 0, Reason: "no embedder configured"}, nil
	}
	actualEmb, err := s.Embedder.Embed(ctx, actual)
	if err != nil {
		return Score{Pass: false, Value: 0, Reason: "embed actual: " + err.Error()}, nil
	}
	expectedEmb, err := s.Embedder.Embed(ctx, expected.Output)
	if err != nil {
		return Score{Pass: false, Value: 0, Reason: "embed expected: " + err.Error()}, nil
	}
	sim := cosineSimilarity(actualEmb, expectedEmb)
	pass := sim >= threshold
	return Score{Pass: pass, Value: sim, Reason: "cosine similarity"}, nil
}

// cosineSimilarity returns the cosine similarity between two vectors (assumed same length).
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
