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

const defaultGeminiBase = "https://generativelanguage.googleapis.com/v1beta"

// GeminiClient is an HTTP client for the Google Gemini API.
type GeminiClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// GeminiConfig configures the Gemini client.
type GeminiConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewGemini creates a Gemini provider.
func NewGemini(cfg GeminiConfig) (*GeminiClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini: API key is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultGeminiBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &GeminiClient{
		BaseURL:    strings.TrimSuffix(base, "/"),
		APIKey:     cfg.APIKey,
		HTTPClient: client,
	}, nil
}

type geminiReq struct {
	Contents         []geminiContent `json:"contents"`
	SystemInstruction *struct {
		Parts []geminiPart `json:"parts"`
	} `json:"systemInstruction,omitempty"`
	GenerationConfig *struct {
		Temperature     float64  `json:"temperature,omitempty"`
		MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
		StopSequences   []string `json:"stopSequences,omitempty"`
	} `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResp struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// Complete implements Provider.
func (c *GeminiClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-1.5-flash"
	}
	body := geminiReq{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: req.Prompt}}}},
	}
	if req.System != "" {
		body.SystemInstruction = &struct {
			Parts []geminiPart `json:"parts"`
		}{Parts: []geminiPart{{Text: req.System}}}
	}
	body.GenerationConfig = &struct {
		Temperature     float64  `json:"temperature,omitempty"`
		MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
		StopSequences   []string `json:"stopSequences,omitempty"`
	}{
		Temperature:     req.Temperature,
		MaxOutputTokens: req.MaxTokens,
		StopSequences:   req.StopTokens,
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("gemini encode: %w", err)
	}
	url := c.BaseURL + "/models/" + model + ":generateContent"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-goog-api-key", c.APIKey)
	httpReq.Header.Set("content-type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini api error %d: %s", resp.StatusCode, string(bs))
	}
	var out geminiResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("gemini decode: %w", err)
	}
	if len(out.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: no candidates")
	}
	var text string
	for _, p := range out.Candidates[0].Content.Parts {
		text += p.Text
	}
	usage := TokenUsage{}
	if out.UsageMetadata != nil {
		usage.PromptTokens = out.UsageMetadata.PromptTokenCount
		usage.CompletionTokens = out.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = out.UsageMetadata.TotalTokenCount
	}
	return &CompletionResponse{
		Content:      text,
		Model:        model,
		Usage:        usage,
		FinishReason: out.Candidates[0].FinishReason,
		Metadata:     req.Metadata,
	}, nil
}

// Stream implements Provider (non-streaming fallback).
func (c *GeminiClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
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
func (c *GeminiClient) GetModelInfo(model string) (*ModelInfo, error) {
	if model == "" {
		model = "gemini-1.5-flash"
	}
	return &ModelInfo{ID: model, ContextSize: 1000000, SupportsStreaming: true}, nil
}
