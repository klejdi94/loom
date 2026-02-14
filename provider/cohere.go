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

const defaultCohereBase = "https://api.cohere.com/v2"

// CohereClient is an HTTP client for the Cohere Chat API.
type CohereClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// CohereConfig configures the Cohere client.
type CohereConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewCohere creates a Cohere provider.
func NewCohere(cfg CohereConfig) (*CohereClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("cohere: API key is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultCohereBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &CohereClient{
		BaseURL:    strings.TrimSuffix(base, "/"),
		APIKey:     cfg.APIKey,
		HTTPClient: client,
	}, nil
}

type cohereMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type cohereReq struct {
	Model       string      `json:"model"`
	Messages    []cohereMsg `json:"messages"`
	Temperature float64     `json:"temperature,omitempty"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Stream      bool        `json:"stream,omitempty"`
}

type cohereResp struct {
	Message *struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Meta *struct {
		BilledUnits *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"billed_units"`
	} `json:"meta"`
}

// Complete implements Provider.
func (c *CohereClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	messages := make([]cohereMsg, 0, 2)
	if req.System != "" {
		messages = append(messages, cohereMsg{Role: "system", Content: req.System})
	}
	messages = append(messages, cohereMsg{Role: "user", Content: req.Prompt})
	body := cohereReq{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
	if body.Model == "" {
		body.Model = "command-r-plus"
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("cohere encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cohere request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cohere api error %d: %s", resp.StatusCode, string(bs))
	}
	var out cohereResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("cohere decode: %w", err)
	}
	if out.Message == nil {
		return nil, fmt.Errorf("cohere: no message")
	}
	usage := TokenUsage{}
	if out.Meta != nil && out.Meta.BilledUnits != nil {
		usage.PromptTokens = out.Meta.BilledUnits.InputTokens
		usage.CompletionTokens = out.Meta.BilledUnits.OutputTokens
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return &CompletionResponse{
		Content:      out.Message.Content,
		Model:        body.Model,
		Usage:        usage,
		FinishReason: "end_turn",
		Metadata:     req.Metadata,
	}, nil
}

// Stream implements Provider (non-streaming fallback).
func (c *CohereClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
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
func (c *CohereClient) GetModelInfo(model string) (*ModelInfo, error) {
	if model == "" {
		model = "command-r-plus"
	}
	return &ModelInfo{ID: model, ContextSize: 128000, SupportsStreaming: true}, nil
}
