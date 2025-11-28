// Package embedding provides embedding generation services.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client provides embedding generation using OpenRouter API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	dimension  int
}

// Config holds embedding client configuration.
type Config struct {
	APIKey    string
	Model     string // e.g., "google/gemini-embedding-001"
	BaseURL   string // Default: https://openrouter.ai/api/v1
	Dimension int    // Default: 768
	Timeout   time.Duration
}

// NewClient creates a new embedding client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://openrouter.ai/api/v1"
	}

	if cfg.Model == "" {
		cfg.Model = "google/gemini-embedding-001"
	}

	if cfg.Dimension <= 0 {
		cfg.Dimension = 768
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		dimension:  cfg.Dimension,
	}, nil
}

// EmbeddingRequest represents a request to generate embeddings.
type EmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

// EmbeddingResponse represents the API response.
type EmbeddingResponse struct {
	Object string           `json:"object"`
	Data   []EmbeddingData  `json:"data"`
	Model  string           `json:"model"`
	Usage  EmbeddingUsage   `json:"usage"`
	Error  *EmbeddingError  `json:"error,omitempty"`
}

// EmbeddingData contains the embedding vector.
type EmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// EmbeddingUsage contains token usage information.
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbeddingError represents an API error.
type EmbeddingError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Embed generates embeddings for the given texts.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := EmbeddingRequest{
		Input: texts,
		Model: c.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("HTTP-Referer", "https://spherical.ai")
	req.Header.Set("X-Title", "Knowledge Engine")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp EmbeddingResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
			return nil, fmt.Errorf("API error: %s (type: %s)", errResp.Error.Message, errResp.Error.Type)
		}
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var embResp EmbeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Sort by index and extract embeddings
	embeddings := make([][]float32, len(texts))
	for _, data := range embResp.Data {
		if data.Index < len(embeddings) {
			embeddings[data.Index] = data.Embedding
			// Update dimension from actual API response
			if len(data.Embedding) > 0 && c.dimension != len(data.Embedding) {
				c.dimension = len(data.Embedding)
			}
		}
	}

	return embeddings, nil
}

// EmbedSingle generates an embedding for a single text.
func (c *Client) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for texts in batches.
func (c *Client) EmbedBatch(ctx context.Context, texts []string, batchSize int) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	embeddings := make([][]float32, 0, len(texts))

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		batchEmbeddings, err := c.Embed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}

		embeddings = append(embeddings, batchEmbeddings...)
	}

	return embeddings, nil
}

// Model returns the model being used.
func (c *Client) Model() string {
	return c.model
}

// Dimension returns the embedding dimension.
func (c *Client) Dimension() int {
	return c.dimension
}

// MockClient provides a mock embedding client for testing.
type MockClient struct {
	dimension int
}

// NewMockClient creates a mock client that generates random embeddings.
func NewMockClient(dimension int) *MockClient {
	if dimension <= 0 {
		dimension = 768
	}
	return &MockClient{dimension: dimension}
}

// Embed generates mock embeddings (zeros for deterministic testing).
func (c *MockClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = make([]float32, c.dimension)
		// Generate simple hash-based embedding for consistency
		for j, char := range texts[i] {
			if j >= c.dimension {
				break
			}
			embeddings[i][j%c.dimension] += float32(char) / 1000.0
		}
		// Normalize
		embeddings[i] = normalize(embeddings[i])
	}
	return embeddings, nil
}

// EmbedSingle generates a mock embedding for a single text.
func (c *MockClient) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

// Model returns the mock model name.
func (c *MockClient) Model() string {
	return "mock-embedding-model"
}

// Dimension returns the embedding dimension.
func (c *MockClient) Dimension() int {
	return c.dimension
}

func normalize(v []float32) []float32 {
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return v
	}
	norm := float32(1.0) / float32(sqrt(float64(sum)))
	for i := range v {
		v[i] *= norm
	}
	return v
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// Embedder defines the interface for embedding generation.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
	Model() string
	Dimension() int
}

// Ensure implementations satisfy interface.
var (
	_ Embedder = (*Client)(nil)
	_ Embedder = (*MockClient)(nil)
)

