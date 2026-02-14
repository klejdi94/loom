package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultOpenAIEmbedBase = "https://api.openai.com/v1"

// OpenAIEmbedder calls the OpenAI embeddings API.
type OpenAIEmbedder struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

// NewOpenAIEmbedder creates an embedder using the OpenAI embeddings API.
func NewOpenAIEmbedder(apiKey string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		APIKey:     apiKey,
		Model:      "text-embedding-3-small",
		BaseURL:    defaultOpenAIEmbedBase,
		HTTPClient: http.DefaultClient,
	}
}

type openAIEmbedReq struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type openAIEmbedResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed implements Embedder.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if e.APIKey == "" {
		return nil, fmt.Errorf("openai embedder: API key required")
	}
	model := e.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	body := openAIEmbedReq{Input: text, Model: model}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/embeddings", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embeddings %d: %s", resp.StatusCode, string(bs))
	}
	var out openAIEmbedResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("openai embeddings: no data")
	}
	return out.Data[0].Embedding, nil
}
