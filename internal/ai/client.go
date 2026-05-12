package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type EmbeddingResponse struct {
	Data []Embedding `json:"data"`
}

type Embedding struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

func NewClient(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) CreateEmbeddings(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	if c.apiKey == "" {
		return EmbeddingResponse{}, fmt.Errorf("openai api key is required")
	}
	if req.Model == "" {
		return EmbeddingResponse{}, fmt.Errorf("embedding model is required")
	}
	if len(req.Input) == 0 {
		return EmbeddingResponse{}, fmt.Errorf("embedding input is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return EmbeddingResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return EmbeddingResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return EmbeddingResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return EmbeddingResponse{}, fmt.Errorf("openai embeddings status: %s", resp.Status)
	}

	var out EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return EmbeddingResponse{}, err
	}
	return out, nil
}
