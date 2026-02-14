// Package middleware provides observability and cross-cutting wrappers for LLM providers.
package middleware

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klejdi94/loom/provider"
)

// Middleware wraps a provider with additional behavior (logging, metrics, cache, etc.).
type Middleware func(provider.Provider) provider.Provider

// Chain wraps p with all middlewares in order (first middleware is outermost).
func Chain(p provider.Provider, mws ...Middleware) provider.Provider {
	for i := len(mws) - 1; i >= 0; i-- {
		p = mws[i](p)
	}
	return p
}

// loggingProvider logs requests and responses.
type loggingProvider struct {
	next provider.Provider
	logf func(format string, args ...interface{})
}

// Logging returns a middleware that logs each Complete call (prompt snippet, model, error).
func Logging(logf func(format string, args ...interface{})) Middleware {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	return func(p provider.Provider) provider.Provider {
		return &loggingProvider{next: p, logf: logf}
	}
}

func (l *loggingProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	l.logf("complete model=%s prompt_len=%d", req.Model, len(req.Prompt))
	resp, err := l.next.Complete(ctx, req)
	if err != nil {
		l.logf("complete error: %v", err)
		return nil, err
	}
	l.logf("complete ok usage=%+v", resp.Usage)
	return resp, nil
}

func (l *loggingProvider) Stream(ctx context.Context, req provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	return l.next.Stream(ctx, req)
}

func (l *loggingProvider) GetModelInfo(model string) (*provider.ModelInfo, error) {
	return l.next.GetModelInfo(model)
}

// metricsProvider counts requests and token usage.
type metricsProvider struct {
	next       provider.Provider
	requests   atomic.Uint64
	errors     atomic.Uint64
	promptTok  atomic.Uint64
	completeTok atomic.Uint64
}

// Metrics returns a middleware that counts requests, errors, and token usage.
// Counters are exposed via Requests, Errors, PromptTokens, CompletionTokens.
func Metrics() (Middleware, *MetricsCounters) {
	m := &metricsProvider{}
	return func(p provider.Provider) provider.Provider {
		m.next = p
		return m
	}, &MetricsCounters{m: m}
}

// MetricsCounters provides read access to collected metrics.
type MetricsCounters struct {
	m *metricsProvider
}

func (c *MetricsCounters) Requests() uint64   { return c.m.requests.Load() }
func (c *MetricsCounters) Errors() uint64     { return c.m.errors.Load() }
func (c *MetricsCounters) PromptTokens() uint64   { return c.m.promptTok.Load() }
func (c *MetricsCounters) CompletionTokens() uint64 { return c.m.completeTok.Load() }

func (m *metricsProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	m.requests.Add(1)
	resp, err := m.next.Complete(ctx, req)
	if err != nil {
		m.errors.Add(1)
		return nil, err
	}
	m.promptTok.Add(uint64(resp.Usage.PromptTokens))
	m.completeTok.Add(uint64(resp.Usage.CompletionTokens))
	return resp, nil
}

func (m *metricsProvider) Stream(ctx context.Context, req provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	return m.next.Stream(ctx, req)
}

func (m *metricsProvider) GetModelInfo(model string) (*provider.ModelInfo, error) {
	return m.next.GetModelInfo(model)
}

// cacheProvider caches Complete responses by (model + system + prompt) key.
type cacheProvider struct {
	next  provider.Provider
	cache Cache
	ttl   time.Duration
}

// Cache is the interface for response caching.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
}

// CacheMiddleware returns a middleware that caches Complete responses. Stream is not cached.
func CacheMiddleware(cache Cache, ttl time.Duration) Middleware {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return func(p provider.Provider) provider.Provider {
		return &cacheProvider{next: p, cache: cache, ttl: ttl}
	}
}

func (c *cacheProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	key := req.Model + "\x00" + req.System + "\x00" + req.Prompt
	if c.cache != nil {
		if raw, ok := c.cache.Get(ctx, key); ok {
			var resp provider.CompletionResponse
			if err := decodeResponse(raw, &resp); err == nil {
				return &resp, nil
			}
		}
	}
	resp, err := c.next.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	if c.cache != nil {
		if raw, err := encodeResponse(resp); err == nil {
			_ = c.cache.Set(ctx, key, raw, c.ttl)
		}
	}
	return resp, nil
}

func (c *cacheProvider) Stream(ctx context.Context, req provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	return c.next.Stream(ctx, req)
}

func (c *cacheProvider) GetModelInfo(model string) (*provider.ModelInfo, error) {
	return c.next.GetModelInfo(model)
}

// InMemoryCache is a simple in-memory cache (for testing/single process).
type InMemoryCache struct {
	mu    sync.RWMutex
	store map[string]cacheEntry
}

type cacheEntry struct {
	val     []byte
	expires time.Time
}

func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{store: make(map[string]cacheEntry)}
}

func (m *InMemoryCache) Get(ctx context.Context, key string) ([]byte, bool) {
	m.mu.RLock()
	e, ok := m.store[key]
	m.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.val, true
}

func (m *InMemoryCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	m.mu.Lock()
	m.store[key] = cacheEntry{val: val, expires: time.Now().Add(ttl)}
	m.mu.Unlock()
	return nil
}

func encodeResponse(r *provider.CompletionResponse) ([]byte, error) {
	return json.Marshal(r)
}
func decodeResponse(raw []byte, r *provider.CompletionResponse) error {
	return json.Unmarshal(raw, r)
}

// rateLimitProvider limits requests per window.
type rateLimitProvider struct {
	next   provider.Provider
	limit  int
	window time.Duration
	tokens chan struct{}
}

// RateLimit returns a middleware that allows at most limit requests per window (e.g. 100 per time.Minute).
func RateLimit(limit int, window time.Duration) Middleware {
	return func(p provider.Provider) provider.Provider {
		r := &rateLimitProvider{next: p, limit: limit, window: window, tokens: make(chan struct{}, limit)}
		for i := 0; i < limit; i++ {
			r.tokens <- struct{}{}
		}
		go func() {
			tick := window / time.Duration(limit)
			if tick < time.Millisecond {
				tick = time.Millisecond
			}
			for range time.Tick(tick) {
				select {
				case r.tokens <- struct{}{}:
				default:
				}
			}
		}()
		return r
	}
}

func (r *rateLimitProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	select {
	case <-r.tokens:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return r.next.Complete(ctx, req)
}

func (r *rateLimitProvider) Stream(ctx context.Context, req provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	return r.next.Stream(ctx, req)
}

func (r *rateLimitProvider) GetModelInfo(model string) (*provider.ModelInfo, error) {
	return r.next.GetModelInfo(model)
}

// circuitBreakerProvider fails fast when error rate is high.
type circuitBreakerProvider struct {
	next      provider.Provider
	threshold float64
	timeout   time.Duration
	requests  atomic.Uint64
	failures  atomic.Uint64
	state     atomic.Uint32 // 0 closed, 1 open, 2 half-open
	openUntil time.Time
	mu        sync.Mutex
}

const (
	cbClosed uint32 = iota
	cbOpen
	cbHalfOpen
)

// CircuitBreaker returns a middleware that opens (fails fast) when failure rate exceeds threshold (e.g. 0.5).
// After timeout it allows one request (half-open); success closes the circuit.
func CircuitBreaker(threshold float64, timeout time.Duration) Middleware {
	return func(p provider.Provider) provider.Provider {
		return &circuitBreakerProvider{next: p, threshold: threshold, timeout: timeout}
	}
}

func (c *circuitBreakerProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	if c.state.Load() == cbOpen {
		c.mu.Lock()
		if time.Now().Before(c.openUntil) {
			c.mu.Unlock()
			return nil, context.DeadlineExceeded
		}
		c.state.Store(cbHalfOpen)
		c.mu.Unlock()
	}
	c.requests.Add(1)
	resp, err := c.next.Complete(ctx, req)
	if err != nil {
		c.failures.Add(1)
		c.mu.Lock()
		if c.state.Load() == cbHalfOpen {
			c.state.Store(cbOpen)
			c.openUntil = time.Now().Add(c.timeout)
		} else if c.requests.Load() >= 10 {
			rate := float64(c.failures.Load()) / float64(c.requests.Load())
			if rate >= c.threshold {
				c.state.Store(cbOpen)
				c.openUntil = time.Now().Add(c.timeout)
			}
		}
		c.mu.Unlock()
		return nil, err
	}
	if c.state.Load() == cbHalfOpen {
		c.state.Store(cbClosed)
	}
	return resp, nil
}

func (c *circuitBreakerProvider) Stream(ctx context.Context, req provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	return c.next.Stream(ctx, req)
}

func (c *circuitBreakerProvider) GetModelInfo(model string) (*provider.ModelInfo, error) {
	return c.next.GetModelInfo(model)
}
