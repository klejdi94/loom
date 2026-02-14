package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultCerebrasBase = "https://api.cerebras.ai/v1"

// CerebrasClient is an HTTP client for the Cerebras inference API (OpenAI-compatible).
type CerebrasClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// CerebrasConfig configures the Cerebras client.
type CerebrasConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewCerebras creates a Cerebras provider.
func NewCerebras(cfg CerebrasConfig) (*CerebrasClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("cerebras: API key is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultCerebrasBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &CerebrasClient{
		BaseURL:    strings.TrimSuffix(base, "/"),
		APIKey:     cfg.APIKey,
		HTTPClient: client,
	}, nil
}

// Cerebras uses OpenAI-compatible request/response.
type cerebrasReq struct {
	Model       string        `json:"model"`
	Messages    []openAIMsg   `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type cerebrasResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Complete implements Provider.
func (c *CerebrasClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	messages := buildMessages(req)
	body := cerebrasReq{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
	if body.Model == "" {
		body.Model = "llama-3.1-70b"
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("cerebras encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cerebras request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cerebras api error %d: %s", resp.StatusCode, string(bs))
	}
	var out cerebrasResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("cerebras decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("cerebras: no choices")
	}
	usage := TokenUsage{}
	if out.Usage != nil {
		usage.PromptTokens = out.Usage.PromptTokens
		usage.CompletionTokens = out.Usage.CompletionTokens
		usage.TotalTokens = out.Usage.TotalTokens
	}
	return &CompletionResponse{
		Content:      out.Choices[0].Message.Content,
		Model:        body.Model,
		Usage:        usage,
		FinishReason: out.Choices[0].FinishReason,
		Metadata:     req.Metadata,
	}, nil
}

// Stream implements Provider (non-streaming fallback).
func (c *CerebrasClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	resp, err := c.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Content: resp.Content, Done: true, Usage: &resp.Usage}
	close(ch)
	return ch, nil
}

// GetModelInfo implements Provider.
func (c *CerebrasClient) GetModelInfo(model string) (*ModelInfo, error) {
	if model == "" {
		model = "llama-3.1-70b"
	}
	return &ModelInfo{ID: model, ContextSize: 32768, SupportsStreaming: true}, nil
}
