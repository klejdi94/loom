// Package optimizer provides A/B testing and optimization.
package optimizer

import (
	"context"
	"math"
	"math/rand"
	"sync"

	"github.com/klejdi94/loom/core"
)

// OnWinnerFunc is called when an experiment has a statistically significant winner (once).
type OnWinnerFunc func(winnerName string, prompt *core.Prompt)

// Experiment represents an A/B test over prompt variants.
type Experiment struct {
	mu               sync.RWMutex
	name             string
	variants         []Variant
	successes        []int64
	totals           []int64
	minSampleSize    int64
	confidenceLevel  float64
	onWinner         OnWinnerFunc
	winnerFired      bool
}

// Variant is one prompt variant in an experiment.
type Variant struct {
	Name   string
	Prompt *core.Prompt
	Weight float64
}

// NewExperiment creates a new experiment with the given name.
func NewExperiment(name string) *Experiment {
	return &Experiment{name: name, confidenceLevel: 0.95}
}

// Variant adds a variant with weight (e.g. 0.5 for 50% traffic). Weights should sum to 1.
func (e *Experiment) Variant(name string, p *core.Prompt, weight float64) *Experiment {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.variants = append(e.variants, Variant{Name: name, Prompt: p, Weight: weight})
	e.successes = append(e.successes, 0)
	e.totals = append(e.totals, 0)
	return e
}

// WithMinSampleSize sets the minimum total events per variant before considering a winner.
func (e *Experiment) WithMinSampleSize(n int64) *Experiment {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.minSampleSize = n
	return e
}

// WithConfidenceLevel sets the required confidence for declaring a winner (e.g. 0.95).
func (e *Experiment) WithConfidenceLevel(c float64) *Experiment {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.confidenceLevel = c
	return e
}

// WithOnWinner sets a callback invoked once when HasWinner becomes true (e.g. to promote the winner in a registry).
func (e *Experiment) WithOnWinner(cb OnWinnerFunc) *Experiment {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onWinner = cb
	return e
}

// Execute runs one variant (selected by weight) and returns the rendered prompt and chosen variant name.
func (e *Experiment) Execute(ctx context.Context, input core.Input) (*core.Rendered, string, error) {
	e.mu.RLock()
	if len(e.variants) == 0 {
		e.mu.RUnlock()
		return nil, "", nil
	}
	weights := make([]float64, len(e.variants))
	for i := range e.variants {
		weights[i] = e.variants[i].Weight
	}
	e.mu.RUnlock()
	idx := selectWeightedIndex(weights)
	e.mu.RLock()
	v := e.variants[idx]
	e.mu.RUnlock()
	rendered, err := v.Prompt.Render(ctx, input)
	if err != nil {
		return nil, "", err
	}
	return rendered, v.Name, nil
}

func selectWeightedIndex(weights []float64) int {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	if sum <= 0 {
		return 0
	}
	r := rand.Float64() * sum
	for i, w := range weights {
		r -= w
		if r <= 0 {
			return i
		}
	}
	return len(weights) - 1
}

// RecordSuccess records an outcome for a variant (e.g. after measuring conversion). success is true for a positive outcome.
// If HasWinner becomes true and WithOnWinner was set, the callback is invoked once.
func (e *Experiment) RecordSuccess(ctx context.Context, variantName string, success bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.variants {
		if e.variants[i].Name == variantName {
			e.totals[i]++
			if success {
				e.successes[i]++
			}
			if !e.winnerFired && e.onWinner != nil {
				if idx, ok := e.winnerLocked(); ok {
					e.winnerFired = true
					name := e.variants[idx].Name
					prompt := e.variants[idx].Prompt
					e.mu.Unlock()
					e.onWinner(name, prompt)
					e.mu.Lock()
				}
			}
			return
		}
	}
}

// HasWinner returns true if min sample size is met and one variant is statistically significantly better.
func (e *Experiment) HasWinner() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.variants) < 2 {
		return false
	}
	for _, t := range e.totals {
		if t < e.minSampleSize {
			return false
		}
	}
	_, ok := e.winnerLocked()
	return ok
}

func (e *Experiment) winnerLocked() (int, bool) {
	bestIdx := -1
	bestRate := -1.0
	for i := range e.variants {
		if e.totals[i] == 0 {
			continue
		}
		rate := float64(e.successes[i]) / float64(e.totals[i])
		if rate > bestRate {
			bestRate = rate
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return 0, false
	}
	// Simple significance: best rate must be above others with margin (approximate z-test).
	for i := range e.variants {
		if i == bestIdx || e.totals[i] == 0 {
			continue
		}
		p2 := float64(e.successes[i]) / float64(e.totals[i])
		se := math.Sqrt(bestRate*(1-bestRate)/float64(e.totals[bestIdx]) + p2*(1-p2)/float64(e.totals[i]))
		if se > 0 && (bestRate-p2)/se < 1.96 {
			return bestIdx, false
		}
	}
	return bestIdx, true
}

// GetWinner returns the name of the winning variant and true if there is one.
func (e *Experiment) GetWinner() (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	idx, ok := e.winnerLocked()
	if !ok {
		return "", false
	}
	return e.variants[idx].Name, true
}

// Promote returns the winning variant's prompt for the caller to promote in the registry.
func (e *Experiment) Promote(winnerName string) *core.Prompt {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for i := range e.variants {
		if e.variants[i].Name == winnerName {
			return e.variants[i].Prompt
		}
	}
	return nil
}

// Stats returns per-variant success counts and totals.
func (e *Experiment) Stats() (names []string, successes, totals []int64) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names = make([]string, len(e.variants))
	successes = make([]int64, len(e.variants))
	totals = make([]int64, len(e.variants))
	for i := range e.variants {
		names[i] = e.variants[i].Name
		successes[i] = e.successes[i]
		totals[i] = e.totals[i]
	}
	return names, successes, totals
}
