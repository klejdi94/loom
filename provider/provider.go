// Package provider defines the LLM provider interface and implementations.
package provider

import (
	"context"
	"io"
)

// CompletionRequest is the unified request for LLM completion.
type CompletionRequest struct {
	Prompt      string
	System      string
	Model       string
	Temperature float64
	MaxTokens   int
	StopTokens  []string
	TopP        float64
	Metadata    map[string]interface{}
}

// CompletionResponse is the unified completion response.
type CompletionResponse struct {
	Content   string
	Model     string
	Usage     TokenUsage
	FinishReason string
	Metadata  map[string]interface{}
}

// TokenUsage reports token counts.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// StreamChunk is a chunk of a streaming response.
type StreamChunk struct {
	Content string
	Done    bool
	Usage   *TokenUsage
	Err     error
}

// ModelInfo describes an LLM model.
type ModelInfo struct {
	ID          string
	ContextSize int
	SupportsStreaming bool
}

// Provider is the unified interface for LLM providers.
type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
	GetModelInfo(model string) (*ModelInfo, error)
}

// StreamToWriter writes stream chunks to w until the channel is closed or context is done.
func StreamToWriter(ctx context.Context, ch <-chan StreamChunk, w io.Writer) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-ch:
			if !ok {
				return nil
			}
			if chunk.Err != nil {
				return chunk.Err
			}
			if _, err := io.WriteString(w, chunk.Content); err != nil {
				return err
			}
			if chunk.Done {
				return nil
			}
		}
	}
}
