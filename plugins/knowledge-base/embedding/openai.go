package kbembedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/copcon/core/llm"
)

// openAIEmbedder implements Embedder using the OpenAI Embeddings API.
// It makes direct HTTP requests to the /v1/embeddings endpoint.
type openAIEmbedder struct {
	llm        llm.LLMProvider
	model      string
	dimensions int
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

const embedBatchSize = 100
const embedMaxRetries = 3

var _ Embedder = (*openAIEmbedder)(nil)

// modelDimensions maps supported OpenAI embedding models to their default vector dimensions.
var modelDimensions = map[string]int{
	"text-embedding-3-small": 1536,
	"text-embedding-3-large": 3072,
	"text-embedding-ada-002": 1536,
}

// openAIEmbedRequest is the JSON body for the OpenAI Embeddings API request.
type openAIEmbedRequest struct {
	Input      any    `json:"input"`
	Model      string `json:"model"`
	Dimensions *int   `json:"dimensions,omitempty"`
}

// openAIEmbedResponse is the JSON body from the OpenAI Embeddings API response.
type openAIEmbedResponse struct {
	Object string            `json:"object"`
	Data   []openAIEmbedData `json:"data"`
	Error  *openAIErrorBody  `json:"error,omitempty"`
}

type openAIEmbedData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type openAIErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// NewOpenAIEmbedder creates an Embedder backed by the OpenAI Embeddings API.
// The baseURL should be the full API base (e.g. "https://api.openai.com/v1/").
// Supported models: text-embedding-3-small, text-embedding-3-large, text-embedding-ada-002.
func NewOpenAIEmbedder(llm llm.LLMProvider, baseURL, apiKey, model string) (Embedder, error) {
	dims, ok := modelDimensions[model]
	if !ok {
		valid := make([]string, 0, len(modelDimensions))
		for m := range modelDimensions {
			valid = append(valid, m)
		}
		return nil, fmt.Errorf("unsupported embedding model %q; supported: %s", model, strings.Join(valid, ", "))
	}

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1/"
	}
	baseURL = strings.TrimRight(baseURL, "/") + "/"

	return &openAIEmbedder{
		llm:        llm,
		model:      model,
		dimensions: dims,
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (e *openAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, ErrEmptyText
	}
	vectors, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding returned empty result")
	}
	return vectors[0], nil
}

func (e *openAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, ErrEmptyText
	}
	for _, t := range texts {
		if t == "" {
			return nil, ErrEmptyText
		}
	}

	totalBatches := (len(texts) + embedBatchSize - 1) / embedBatchSize
	slog.Debug("embedding request (batched)", "model", e.model, "base_url", e.baseURL, "texts_count", len(texts), "batch_size", embedBatchSize, "total_batches", totalBatches)

	allEmbeddings := make([][]float32, 0, len(texts))

	for batchIdx := 0; batchIdx < totalBatches; batchIdx++ {
		start := batchIdx * embedBatchSize
		end := start + embedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchTexts := texts[start:end]

		slog.Debug("embedding batch", "batch", fmt.Sprintf("%d/%d", batchIdx+1, totalBatches), "count", len(batchTexts))

		embeddings, err := e.embedSingleBatch(ctx, batchTexts)
		if err != nil {
			return nil, fmt.Errorf("batch %d/%d: %w", batchIdx+1, totalBatches, err)
		}

		allEmbeddings = append(allEmbeddings, embeddings...)
	}

	return allEmbeddings, nil
}

func (e *openAIEmbedder) embedSingleBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var lastErr error
	for attempt := 0; attempt < embedMaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			slog.Warn("embedding retry", "attempt", attempt+1, "max_retries", embedMaxRetries, "backoff", backoff, "count", len(texts))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		vectors, err := e.doEmbedRequest(ctx, texts)
		if err == nil {
			return vectors, nil
		}
		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", embedMaxRetries, lastErr)
}

func (e *openAIEmbedder) doEmbedRequest(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openAIEmbedRequest{
		Input: texts,
		Model: e.model,
	}

	if e.model != "text-embedding-ada-002" {
		dims := e.dimensions
		reqBody.Dimensions = &dims
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := e.baseURL + "embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		apiMsg := e.extractAPIError(respBody)
		errMsg := fmt.Sprintf("openai API error (status %d)", resp.StatusCode)
		if apiMsg != "" {
			errMsg += ": " + apiMsg
		}
		slog.Error("embedding API error", "status", resp.StatusCode, "body", string(respBody))
		return nil, &embedHTTPError{StatusCode: resp.StatusCode, Message: errMsg}
	}

	var result openAIEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Error != nil && result.Error.Message != "" {
		return nil, fmt.Errorf("openai API error: %s", result.Error.Message)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embedding returned empty data: %d texts requested, model=%s, status=%d", len(texts), e.model, resp.StatusCode)
	}

	embeddings := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		if d.Index >= 0 && d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}

	for i, emb := range embeddings {
		if len(emb) != e.dimensions {
			return nil, fmt.Errorf("%w: index %d got %d dimensions, expected %d",
				ErrDimensionMismatch, i, len(emb), e.dimensions)
		}
	}

	return embeddings, nil
}

func (e *openAIEmbedder) Dimensions() int {
	return e.dimensions
}

func (e *openAIEmbedder) Name() string {
	return "openai:" + e.model
}

type embedHTTPError struct {
	StatusCode int
	Message    string
}

func (e *embedHTTPError) Error() string {
	return e.Message
}

func isRetryable(err error) bool {
	var httpErr *embedHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500
	}
	msg := err.Error()
	return strings.Contains(msg, "http request:") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "timeout")
}

func (e *openAIEmbedder) extractAPIError(body []byte) string {
	var errResp struct {
		Error *openAIErrorBody `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		return ""
	}
	if errResp.Error != nil && errResp.Error.Message != "" {
		return errResp.Error.Message
	}
	return ""
}