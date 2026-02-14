// Package executor runs prompts against LLM providers with retry and optional middleware.
package executor

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/klejdi94/loom/core"
	"github.com/klejdi94/loom/provider"
)

// Executor executes prompts via a provider (with optional retry).
type Executor struct {
	Provider    provider.Provider
	MaxRetries  int
	Backoff     BackoffFunc
	BaseTimeout time.Duration
}

// BackoffFunc returns delay before the next retry (attempt is 0-based).
type BackoffFunc func(attempt int) time.Duration

// ExponentialBackoff returns delay = base * 2^attempt, capped at max.
func ExponentialBackoff(base, max time.Duration) BackoffFunc {
	return func(attempt int) time.Duration {
		d := base * time.Duration(math.Pow(2, float64(attempt)))
		if d > max {
			return max
		}
		return d
	}
}

// ExecutorOption configures the executor.
type ExecutorOption func(*Executor)

// WithRetry sets max retries and backoff.
func WithRetry(maxRetries int, backoff BackoffFunc) ExecutorOption {
	return func(e *Executor) {
		e.MaxRetries = maxRetries
		e.Backoff = backoff
	}
}

// WithTimeout sets a default request timeout.
func WithTimeout(d time.Duration) ExecutorOption {
	return func(e *Executor) {
		e.BaseTimeout = d
	}
}

// New creates an executor that uses the given provider.
func New(p provider.Provider, opts ...ExecutorOption) *Executor {
	e := &Executor{
		Provider:   p,
		MaxRetries: 0,
		Backoff:    ExponentialBackoff(500*time.Millisecond, 30*time.Second),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// ExecuteRequest holds options for a single completion.
type ExecuteRequest struct {
	Prompt      *core.Prompt
	Input       core.Input
	Model       string
	Temperature float64
	MaxTokens   int
	StopTokens  []string
	Timeout     time.Duration
}

// ExecuteResult is the result of executing a prompt.
type ExecuteResult struct {
	Content   string
	Usage     provider.TokenUsage
	Model     string
	Rendered  *core.Rendered
	Attempts  int
}

// Execute renders the prompt and calls the provider, with retries on failure.
func (e *Executor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResult, error) {
	if req.Prompt == nil {
		return nil, fmt.Errorf("executor: prompt is required")
	}
	rendered, err := req.Prompt.Render(ctx, req.Input)
	if err != nil {
		return nil, fmt.Errorf("executor render: %w", err)
	}
	timeout := req.Timeout
	if timeout == 0 {
		timeout = e.BaseTimeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	creq := provider.CompletionRequest{
		Prompt:      rendered.User,
		System:      rendered.System,
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		StopTokens:  req.StopTokens,
		Metadata:    req.Prompt.Metadata,
	}
	if creq.Model == "" {
		creq.Model = "gpt-3.5-turbo"
	}
	var lastErr error
	attempts := 0
	for attempt := 0; attempt <= e.MaxRetries; attempt++ {
		attempts++
		resp, err := e.Provider.Complete(ctx, creq)
		if err == nil {
			return &ExecuteResult{
				Content:  resp.Content,
				Usage:    resp.Usage,
				Model:    resp.Model,
				Rendered: rendered,
				Attempts: attempts,
			}, nil
		}
		lastErr = err
		if attempt == e.MaxRetries {
			break
		}
		if e.Backoff != nil {
			time.Sleep(e.Backoff(attempt))
		}
	}
	return nil, fmt.Errorf("executor after %d attempts: %w", attempts, lastErr)
}
