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

const defaultAnthropicBase = "https://api.anthropic.com/v1"

// AnthropicClient is an HTTP client for the Anthropic Messages API.
type AnthropicClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// AnthropicConfig configures the Anthropic client.
type AnthropicConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewAnthropic creates an Anthropic provider.
func NewAnthropic(cfg AnthropicConfig) (*AnthropicClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: API key is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultAnthropicBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &AnthropicClient{
		BaseURL:    strings.TrimSuffix(base, "/"),
		APIKey:     cfg.APIKey,
		HTTPClient: client,
	}, nil
}

type anthropicReq struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMsg     `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResp struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason  string `json:"stop_reason"`
	Model       string `json:"model"`
	Usage       *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Complete implements Provider.
func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	body := anthropicReq{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  []anthropicMsg{{Role: "user", Content: req.Prompt}},
		Temperature: req.Temperature,
	}
	if body.Model == "" {
		body.Model = "claude-3-5-sonnet-20241022"
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = 1024
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("anthropic encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/messages", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic api error %d: %s", resp.StatusCode, string(bs))
	}
	var out anthropicResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("anthropic decode: %w", err)
	}
	var text string
	for _, block := range out.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	usage := TokenUsage{}
	if out.Usage != nil {
		usage.PromptTokens = out.Usage.InputTokens
		usage.CompletionTokens = out.Usage.OutputTokens
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return &CompletionResponse{
		Content:      text,
		Model:        out.Model,
		Usage:        usage,
		FinishReason: out.StopReason,
		Metadata:     req.Metadata,
	}, nil
}

// Stream implements Provider (non-streaming fallback for simplicity).
func (c *AnthropicClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
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
func (c *AnthropicClient) GetModelInfo(model string) (*ModelInfo, error) {
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	return &ModelInfo{ID: model, ContextSize: 200000, SupportsStreaming: true}, nil
}
