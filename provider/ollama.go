package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultOllamaBase = "http://localhost:11434"

// OllamaClient is an HTTP client for the Ollama local API.
type OllamaClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// OllamaConfig configures the Ollama client.
type OllamaConfig struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewOllama creates an Ollama provider (no API key required).
func NewOllama(cfg OllamaConfig) *OllamaClient {
	base := cfg.BaseURL
	if base == "" {
		base = defaultOllamaBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &OllamaClient{
		BaseURL:    strings.TrimSuffix(base, "/"),
		HTTPClient: client,
	}
}

type ollamaReq struct {
	Model       string     `json:"model"`
	Messages    []ollamaMsg `json:"messages"`
	Stream      bool       `json:"stream"`
	Options     *struct {
		Temperature float64 `json:"temperature,omitempty"`
		NumPredict  int     `json:"num_predict,omitempty"`
	} `json:"options,omitempty"`
}

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResp struct {
	Message struct {
		Content string `json:"content"`
		Role    string `json:"role"`
	} `json:"message"`
	Done       bool `json:"done"`
	EvalCount  int  `json:"eval_count,omitempty"`
	PromptEvalCount int `json:"prompt_eval_count,omitempty"`
}

func buildOllamaMessages(req CompletionRequest) []ollamaMsg {
	var out []ollamaMsg
	if req.System != "" {
		out = append(out, ollamaMsg{Role: "system", Content: req.System})
	}
	out = append(out, ollamaMsg{Role: "user", Content: req.Prompt})
	return out
}

// Complete implements Provider.
func (c *OllamaClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	messages := buildOllamaMessages(req)
	body := ollamaReq{
		Model:    req.Model,
		Messages: messages,
		Stream:   false,
	}
	if body.Model == "" {
		body.Model = "llama2"
	}
	if req.Temperature != 0 || req.MaxTokens != 0 {
		body.Options = &struct {
			Temperature float64 `json:"temperature,omitempty"`
			NumPredict  int     `json:"num_predict,omitempty"`
		}{Temperature: req.Temperature, NumPredict: req.MaxTokens}
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("ollama encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama api error %d: %s", resp.StatusCode, string(bs))
	}
	var out ollamaResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama decode: %w", err)
	}
	usage := TokenUsage{}
	if out.PromptEvalCount > 0 || out.EvalCount > 0 {
		usage.PromptTokens = out.PromptEvalCount
		usage.CompletionTokens = out.EvalCount
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return &CompletionResponse{
		Content:      out.Message.Content,
		Model:        body.Model,
		Usage:        usage,
		FinishReason: "stop",
		Metadata:     req.Metadata,
	}, nil
}

// Stream implements Provider.
func (c *OllamaClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	messages := buildOllamaMessages(req)
	body := ollamaReq{
		Model:    req.Model,
		Messages: messages,
		Stream:   true,
	}
	if body.Model == "" {
		body.Model = "llama2"
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("ollama encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama api error %d: %s", resp.StatusCode, string(bs))
	}
	ch := make(chan StreamChunk, 8)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			var chunk ollamaResp
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}
			if chunk.Message.Content != "" {
				ch <- StreamChunk{Content: chunk.Message.Content}
			}
			if chunk.Done {
				ch <- StreamChunk{Done: true}
				return
			}
		}
	}()
	return ch, nil
}

// GetModelInfo implements Provider.
func (c *OllamaClient) GetModelInfo(model string) (*ModelInfo, error) {
	if model == "" {
		model = "llama2"
	}
	return &ModelInfo{
		ID:                 model,
		ContextSize:        4096,
		SupportsStreaming:  true,
	}, nil
}
