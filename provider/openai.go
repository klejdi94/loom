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

const defaultOpenAIBase = "https://api.openai.com/v1"

// OpenAIClient is an HTTP client for the OpenAI API.
type OpenAIClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// OpenAIConfig configures the OpenAI client.
type OpenAIConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewOpenAI creates an OpenAI provider.
func NewOpenAI(cfg OpenAIConfig) (*OpenAIClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai: API key is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultOpenAIBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &OpenAIClient{
		BaseURL:    strings.TrimSuffix(base, "/"),
		APIKey:     cfg.APIKey,
		HTTPClient: client,
	}, nil
}

// openAI request/response types (minimal for chat completions).
type openAIChatReq struct {
	Model       string        `json:"model"`
	Messages    []openAIMsg   `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type openAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResp struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      openAIMsg `json:"message"`
		FinishReason string    `json:"finish_reason"`
		Index        int       `json:"index"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Complete implements Provider.
func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	messages := buildMessages(req)
	body := openAIChatReq{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stop:        req.StopTokens,
		Stream:      false,
	}
	if body.Model == "" {
		body.Model = "gpt-3.5-turbo"
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("openai encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai api error %d: %s", resp.StatusCode, string(bs))
	}
	var out openAIChatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("openai decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}
	usage := TokenUsage{}
	if out.Usage != nil {
		usage.PromptTokens = out.Usage.PromptTokens
		usage.CompletionTokens = out.Usage.CompletionTokens
		usage.TotalTokens = out.Usage.TotalTokens
	}
	return &CompletionResponse{
		Content:      out.Choices[0].Message.Content,
		Model:        out.Model,
		Usage:        usage,
		FinishReason: out.Choices[0].FinishReason,
		Metadata:     req.Metadata,
	}, nil
}

// Stream implements Provider.
func (c *OpenAIClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	messages := buildMessages(req)
	body := openAIChatReq{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stop:        req.StopTokens,
		Stream:      true,
	}
	if body.Model == "" {
		body.Model = "gpt-3.5-turbo"
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("openai encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai api error %d: %s", resp.StatusCode, string(bs))
	}
	ch := make(chan StreamChunk, 8)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- StreamChunk{Done: true}
				return
			}
			var block struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &block); err != nil {
				ch <- StreamChunk{Err: err}
				return
			}
			if len(block.Choices) > 0 && block.Choices[0].Delta.Content != "" {
				ch <- StreamChunk{Content: block.Choices[0].Delta.Content}
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Err: err}
		}
	}()
	return ch, nil
}

// GetModelInfo implements Provider (returns a minimal info).
func (c *OpenAIClient) GetModelInfo(model string) (*ModelInfo, error) {
	// Common models; could be extended with an API call.
	info := &ModelInfo{ID: model, SupportsStreaming: true}
	switch {
	case strings.HasPrefix(model, "gpt-4"):
		info.ContextSize = 128000
	case strings.HasPrefix(model, "gpt-3.5"):
		info.ContextSize = 16385
	default:
		info.ContextSize = 8192
	}
	return info, nil
}

func buildMessages(req CompletionRequest) []openAIMsg {
	var messages []openAIMsg
	if req.System != "" {
		messages = append(messages, openAIMsg{Role: "system", Content: req.System})
	}
	messages = append(messages, openAIMsg{Role: "user", Content: req.Prompt})
	return messages
}
