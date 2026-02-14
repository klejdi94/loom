// Package cost provides token counting and cost estimation for LLM requests.
package cost

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/klejdi94/loom/core"
	"github.com/klejdi94/loom/provider"
)

// Estimator estimates cost for a prompt (and optional expected output size).
type Estimator struct {
	model        string
	inputPer1K   float64
	outputPer1K  float64
	tokenCounter TokenCounter
}

// TokenCounter estimates token count for text (e.g. ~4 chars per token for English).
type TokenCounter interface {
	CountTokens(text string) int
}

// SimpleCounter uses a rough heuristic: tokens ≈ runes/4.
type SimpleCounter struct{}

func (SimpleCounter) CountTokens(text string) int {
	n := 0
	for range text {
		n++
	}
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}

// EstimatorOption configures the estimator.
type EstimatorOption func(*Estimator)

// WithTokenCounter sets a custom token counter.
func WithTokenCounter(tc TokenCounter) EstimatorOption {
	return func(e *Estimator) {
		e.tokenCounter = tc
	}
}

// NewEstimator creates an estimator for a model with given pricing (per 1K tokens, USD).
func NewEstimator(model string, inputPer1K, outputPer1K float64, opts ...EstimatorOption) *Estimator {
	e := &Estimator{
		model:       model,
		inputPer1K:  inputPer1K,
		outputPer1K: outputPer1K,
		tokenCounter: SimpleCounter{},
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Estimate returns the estimated cost in USD for rendering the prompt and expected output tokens.
func (e *Estimator) Estimate(ctx context.Context, rendered *core.Rendered, expectedOutputTokens int) (inputCost, outputCost, totalUSD float64) {
	inputTokens := 0
	if e.tokenCounter != nil {
		inputTokens = e.tokenCounter.CountTokens(rendered.System) + e.tokenCounter.CountTokens(rendered.User)
	}
	inputCost = (float64(inputTokens) / 1000) * e.inputPer1K
	outputCost = (float64(expectedOutputTokens) / 1000) * e.outputPer1K
	totalUSD = inputCost + outputCost
	return inputCost, outputCost, totalUSD
}

// Tracker records cost per request (e.g. from actual usage in CompletionResponse).
type Tracker struct {
	totalInputTokens  atomic.Uint64
	totalOutputTokens atomic.Uint64
	mu                sync.Mutex
	totalCostUSD      float64
	modelPricing      map[string]struct{ in, out float64 }
}

// NewTracker creates a cost tracker. Register model pricing with RegisterModel.
func NewTracker() *Tracker {
	return &Tracker{modelPricing: make(map[string]struct{ in, out float64 })}
}

// RegisterModel sets pricing (per 1K tokens) for a model.
func (t *Tracker) RegisterModel(model string, inputPer1K, outputPer1K float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.modelPricing[model] = struct{ in, out float64 }{inputPer1K, outputPer1K}
}

// Record records usage from a completion response and returns the cost in USD.
func (t *Tracker) Record(model string, usage provider.TokenUsage) float64 {
	t.totalInputTokens.Add(uint64(usage.PromptTokens))
	t.totalOutputTokens.Add(uint64(usage.CompletionTokens))
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.modelPricing[model]
	if !ok {
		return 0
	}
	cost := (float64(usage.PromptTokens)/1000)*p.in + (float64(usage.CompletionTokens)/1000)*p.out
	t.totalCostUSD += cost
	return cost
}

// TotalInputTokens returns total prompt tokens recorded.
func (t *Tracker) TotalInputTokens() uint64 {
	return t.totalInputTokens.Load()
}

// TotalOutputTokens returns total completion tokens recorded.
func (t *Tracker) TotalOutputTokens() uint64 {
	return t.totalOutputTokens.Load()
}

// TotalCostUSD returns total cost in USD.
func (t *Tracker) TotalCostUSD() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totalCostUSD
}
