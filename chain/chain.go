// Package chain provides multi-step prompt chains with sequential/parallel execution.
package chain

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/klejdi94/loom/core"
	"github.com/klejdi94/loom/executor"
)

// ChainResult holds outputs from chain steps (keyed by step name).
type ChainResult struct {
	outputs map[string]string
}

// Get returns the output of a step by name.
func (c *ChainResult) Get(step string) string {
	if c.outputs == nil {
		return ""
	}
	return c.outputs[step]
}

// All returns a copy of all step outputs.
func (c *ChainResult) All() map[string]string {
	if c.outputs == nil {
		return nil
	}
	m := make(map[string]string, len(c.outputs))
	for k, v := range c.outputs {
		m[k] = v
	}
	return m
}

// StepOption configures a chain step.
type StepOption func(*stepDef)

// WithRetry sets retry count and backoff for this step.
func WithRetry(maxRetries int, backoff executor.BackoffFunc) StepOption {
	return func(s *stepDef) {
		s.maxRetries = maxRetries
		s.backoff = backoff
	}
}

// WithTimeout sets a per-step timeout.
func WithTimeout(d time.Duration) StepOption {
	return func(s *stepDef) {
		s.timeout = d
	}
}

// WithFallback sets a fallback prompt used when the main step fails after retries.
func WithFallback(p *core.Prompt) StepOption {
	return func(s *stepDef) {
		s.fallback = p
	}
}

// WithCondition runs this step only when the condition returns true (given current chain result).
func WithCondition(cond func(ctx context.Context, result *ChainResult) bool) StepOption {
	return func(s *stepDef) {
		s.condition = cond
	}
}

type stepDef struct {
	name       string
	prompt     *core.Prompt
	maxRetries int
	backoff    executor.BackoffFunc
	timeout    time.Duration
	fallback   *core.Prompt
	condition  func(ctx context.Context, result *ChainResult) bool
}

// StepDef is a step definition for use in Parallel. Create with ChainStep.
type StepDef struct {
	Name       string
	Prompt     *core.Prompt
	MaxRetries int
	Backoff    executor.BackoffFunc
	Timeout    time.Duration
	Fallback   *core.Prompt
	Condition  func(ctx context.Context, result *ChainResult) bool
}

func (s StepDef) toInternal() stepDef {
	return stepDef{
		name: s.Name, prompt: s.Prompt, maxRetries: s.MaxRetries, backoff: s.Backoff,
		timeout: s.Timeout, fallback: s.Fallback, condition: s.Condition,
	}
}

// node is either a single step or a parallel group.
type node struct {
	parallel bool
	steps    []stepDef
}

// Chain represents a multi-step prompt flow.
type Chain struct {
	name     string
	nodes    []node
	exec     *executor.Executor
	defaultModel string
}

// NewChain creates a new chain with the given name.
func NewChain(name string) *Chain {
	return &Chain{name: name}
}

// WithExecutor sets the executor used to run steps (LLM calls). If nil, steps are render-only.
func (c *Chain) WithExecutor(e *executor.Executor) *Chain {
	c.exec = e
	return c
}

// WithDefaultModel sets the model used for each step when no model is set on the executor request.
func (c *Chain) WithDefaultModel(model string) *Chain {
	c.defaultModel = model
	return c
}

// Step adds a sequential step.
func (c *Chain) Step(name string, p *core.Prompt, opts ...StepOption) *Chain {
	s := stepDef{name: name, prompt: p}
	for _, o := range opts {
		o(&s)
	}
	c.nodes = append(c.nodes, node{parallel: false, steps: []stepDef{s}})
	return c
}

// Parallel adds a group of steps that run in parallel (same input, outputs merged).
// Use ChainStep to build each step: Parallel(ChainStep("a", promptA), ChainStep("b", promptB)).
func (c *Chain) Parallel(steps ...StepDef) *Chain {
	if len(steps) == 0 {
		return c
	}
	defs := make([]stepDef, len(steps))
	for i := range steps {
		defs[i] = steps[i].toInternal()
	}
	c.nodes = append(c.nodes, node{parallel: true, steps: defs})
	return c
}

// ChainStep returns a step definition for use in Parallel.
func ChainStep(name string, p *core.Prompt, opts ...StepOption) StepDef {
	s := stepDef{name: name, prompt: p}
	for _, o := range opts {
		o(&s)
	}
	return StepDef{
		Name: s.name, Prompt: s.prompt, MaxRetries: s.maxRetries, Backoff: s.backoff,
		Timeout: s.timeout, Fallback: s.fallback, Condition: s.condition,
	}
}

// Execute runs the chain with the given input. If an executor is set, each step is run through the LLM; otherwise only rendering is performed.
func (c *Chain) Execute(ctx context.Context, input core.Input) (*ChainResult, error) {
	result := &ChainResult{outputs: make(map[string]string)}
	currentInput := make(core.Input)
	for k, v := range input {
		currentInput[k] = v
	}
	for _, n := range c.nodes {
		if n.parallel {
			outputs, err := c.runParallel(ctx, n.steps, currentInput, result)
			if err != nil {
				return nil, err
			}
			for k, v := range outputs {
				result.outputs[k] = v
				currentInput[k] = v
			}
		} else {
			for _, s := range n.steps {
				if s.condition != nil && !s.condition(ctx, result) {
					continue
				}
				out, err := c.runStep(ctx, &s, currentInput)
				if err != nil {
					return nil, fmt.Errorf("chain step %q: %w", s.name, err)
				}
				result.outputs[s.name] = out
				currentInput[s.name] = out
			}
		}
	}
	return result, nil
}

func (c *Chain) runStep(ctx context.Context, s *stepDef, input core.Input) (string, error) {
	timeout := s.timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if c.exec != nil {
		req := executor.ExecuteRequest{
			Prompt: s.prompt, Input: input, Timeout: timeout,
		}
		if c.defaultModel != "" {
			req.Model = c.defaultModel
		}
		// Retry loop
		var lastErr error
		for attempt := 0; attempt <= s.maxRetries; attempt++ {
			res, err := c.exec.Execute(ctx, req)
			if err == nil {
				return res.Content, nil
			}
			lastErr = err
			if attempt == s.maxRetries {
				if s.fallback != nil {
					req.Prompt = s.fallback
					res, err := c.exec.Execute(ctx, req)
					if err != nil {
						return "", fmt.Errorf("step and fallback failed: %w", lastErr)
					}
					return res.Content, nil
				}
				return "", lastErr
			}
			if s.backoff != nil {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(s.backoff(attempt)):
				}
			}
		}
	}
	// Render only
	rendered, err := s.prompt.Render(ctx, input)
	if err != nil {
		return "", err
	}
	return rendered.User, nil
}

func (c *Chain) runParallel(ctx context.Context, steps []stepDef, input core.Input, result *ChainResult) (map[string]string, error) {
	type pair struct {
		name string
		val  string
		err  error
	}
	out := make(map[string]string)
	var wg sync.WaitGroup
	ch := make(chan pair, len(steps))
	for _, s := range steps {
		if s.condition != nil && !s.condition(ctx, result) {
			continue
		}
		wg.Add(1)
		go func(s stepDef) {
			defer wg.Done()
			val, err := c.runStep(ctx, &s, input)
			ch <- pair{s.name, val, err}
		}(s)
	}
	wg.Wait()
	close(ch)
	for p := range ch {
		if p.err != nil {
			return nil, p.err
		}
		out[p.name] = p.val
	}
	return out, nil
}

// Backoff is a convenience for chain steps (re-export or define).
func ExponentialBackoff(base, max time.Duration) executor.BackoffFunc {
	return func(attempt int) time.Duration {
		d := base * time.Duration(math.Pow(2, float64(attempt)))
		if d > max {
			return max
		}
		return d
	}
}
