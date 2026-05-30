package kbembedding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/copcon/core/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// embedTestHandler builds an httptest.Server that responds to /embeddings.
// It returns the server and a channel to inspect the last received request body.
func embedTestHandler(handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(handler))
}

func validEmbedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openAIEmbedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}

	var inputs []string
	switch v := req.Input.(type) {
	case string:
		inputs = []string{v}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				http.Error(w, "non-string input", http.StatusBadRequest)
				return
			}
			inputs = append(inputs, s)
		}
	default:
		http.Error(w, "unexpected input type", http.StatusBadRequest)
		return
	}

	dim := 1536
	if req.Model == "text-embedding-3-large" {
		dim = 3072
	}
	if req.Dimensions != nil {
		dim = *req.Dimensions
	}

	data := make([]openAIEmbedData, len(inputs))
	for i := range inputs {
		vec := make([]float32, dim)
		for j := range vec {
			vec[j] = float32(i*10+j) / float32(dim)
		}
		data[i] = openAIEmbedData{
			Object:    "embedding",
			Index:     i,
			Embedding: vec,
		}
	}

	resp := openAIEmbedResponse{
		Object: "list",
		Data:   data,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func newTestLLMProvider() llm.LLMProvider {
	return &llm.MockProvider{}
}

func TestNewOpenAIEmbedder_ValidModels(t *testing.T) {
	llmProv := newTestLLMProvider()

	tests := []struct {
		model      string
		wantDims   int
		wantName   string
	}{
		{"text-embedding-3-small", 1536, "openai:text-embedding-3-small"},
		{"text-embedding-3-large", 3072, "openai:text-embedding-3-large"},
		{"text-embedding-ada-002", 1536, "openai:text-embedding-ada-002"},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			e, err := NewOpenAIEmbedder(llmProv, "", "test-key", tc.model)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDims, e.Dimensions())
			assert.Equal(t, tc.wantName, e.Name())
		})
	}
}

func TestNewOpenAIEmbedder_InvalidModel(t *testing.T) {
	llmProv := newTestLLMProvider()
	_, err := NewOpenAIEmbedder(llmProv, "", "test-key", "invalid-model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported embedding model")
}

func TestOpenAIEmbedder_Embed(t *testing.T) {
	srv := embedTestHandler(validEmbedHandler)
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	vec, err := e.Embed(context.Background(), "hello world")
	require.NoError(t, err)
	require.Len(t, vec, 1536)
	assert.InDelta(t, 0.0, vec[0], 0.01)
}

func TestOpenAIEmbedder_EmbedBatch(t *testing.T) {
	srv := embedTestHandler(validEmbedHandler)
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	texts := []string{"first", "second", "third"}
	vecs, err := e.EmbedBatch(context.Background(), texts)
	require.NoError(t, err)
	require.Len(t, vecs, 3)
	for i, vec := range vecs {
		require.Len(t, vec, 1536)
		assert.InDelta(t, float32(i*10)/1536, vec[0], 0.01)
	}
}

func TestOpenAIEmbedder_Embed_EmptyText(t *testing.T) {
	srv := embedTestHandler(validEmbedHandler)
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	_, err = e.Embed(context.Background(), "")
	assert.ErrorIs(t, err, ErrEmptyText)
}

func TestOpenAIEmbedder_EmbedBatch_EmptySlice(t *testing.T) {
	srv := embedTestHandler(validEmbedHandler)
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	_, err = e.EmbedBatch(context.Background(), []string{})
	assert.ErrorIs(t, err, ErrEmptyText)
}

func TestOpenAIEmbedder_EmbedBatch_EmptyStringInSlice(t *testing.T) {
	srv := embedTestHandler(validEmbedHandler)
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	_, err = e.EmbedBatch(context.Background(), []string{"hello", "", "world"})
	assert.ErrorIs(t, err, ErrEmptyText)
}

func TestOpenAIEmbedder_HTTP500(t *testing.T) {
	srv := embedTestHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"internal error","type":"server_error","code":"500"}}`))
	})
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	_, err = e.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal error")
}

func TestOpenAIEmbedder_Timeout(t *testing.T) {
	srv := embedTestHandler(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		validEmbedHandler(w, r)
	})
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = e.Embed(ctx, "hello")
	require.Error(t, err)
}

func TestOpenAIEmbedder_DimensionMismatch(t *testing.T) {
	srv := embedTestHandler(func(w http.ResponseWriter, r *http.Request) {
		// Return vectors with wrong dimension (4 instead of 1536).
		resp := openAIEmbedResponse{
			Object: "list",
			Data: []openAIEmbedData{
				{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2, 0.3, 0.4}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	_, err = e.Embed(context.Background(), "hello")
	assert.ErrorIs(t, err, ErrDimensionMismatch)
}

func TestOpenAIEmbedder_InvalidJSONResponse(t *testing.T) {
	srv := embedTestHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	})
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)

	_, err = e.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal response")
}

func TestOpenAIEmbedder_Ada002NoDimensions(t *testing.T) {
	srv := embedTestHandler(func(w http.ResponseWriter, r *http.Request) {
		var req openAIEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Dimensions != nil {
			http.Error(w, "ada-002 should not have dimensions", http.StatusBadRequest)
			return
		}
		// Build response inline (body already consumed above).
		dim := 1536
		vec := make([]float32, dim)
		resp := openAIEmbedResponse{
			Object: "list",
			Data: []openAIEmbedData{
				{Object: "embedding", Index: 0, Embedding: vec},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	e, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-ada-002")
	require.NoError(t, err)

	vec, err := e.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Len(t, vec, 1536)
}

func TestNewFromConfig_OpenAI(t *testing.T) {
	srv := embedTestHandler(validEmbedHandler)
	defer srv.Close()

	cfg := EmbeddingConfig{
		Backend:     BackendOpenAI,
		OpenAIModel: "text-embedding-3-small",
		BaseURL:     srv.URL,
		APIKey:      "test-key",
	}

	e, err := NewFromConfig(cfg, newTestLLMProvider())
	require.NoError(t, err)
	assert.Equal(t, 1536, e.Dimensions())
	assert.Equal(t, "openai:text-embedding-3-small", e.Name())

	vec, err := e.Embed(context.Background(), "test")
	require.NoError(t, err)
	assert.Len(t, vec, 1536)
}

func TestNewFromConfig_BGEM3(t *testing.T) {
	cfg := EmbeddingConfig{
		Backend: BackendBGEM3,
	}

	_, err := NewFromConfig(cfg, newTestLLMProvider())
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrUnsupportedBackend.Error())
}

func TestNewFromConfig_UnknownBackend(t *testing.T) {
	cfg := EmbeddingConfig{
		Backend: "unknown",
	}

	_, err := NewFromConfig(cfg, newTestLLMProvider())
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrUnsupportedBackend.Error())
}

func TestOpenAIEmbedder_CompileTimeCheck(t *testing.T) {
	var _ Embedder = (*openAIEmbedder)(nil)
}

func TestOpenAIEmbedder_ModelValidationErrors(t *testing.T) {
	llmProv := newTestLLMProvider()

	// Empty model string.
	_, err := NewOpenAIEmbedder(llmProv, "", "key", "")
	require.Error(t, err)

	// Model with wrong casing.
	_, err = NewOpenAIEmbedder(llmProv, "", "key", "TEXT-EMBEDDING-3-SMALL")
	require.Error(t, err)
}

func TestOpenAIEmbedder_BaseURLTrailingSlash(t *testing.T) {
	srv := embedTestHandler(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path ends with /embeddings.
		if !strings.HasSuffix(r.URL.Path, "/embeddings") {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		validEmbedHandler(w, r)
	})
	defer srv.Close()

	// Test with trailing slash.
	e1, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL+"/", "test-key", "text-embedding-3-small")
	require.NoError(t, err)
	_, err = e1.Embed(context.Background(), "hello")
	require.NoError(t, err)

	// Test without trailing slash.
	e2, err := NewOpenAIEmbedder(newTestLLMProvider(), srv.URL, "test-key", "text-embedding-3-small")
	require.NoError(t, err)
	_, err = e2.Embed(context.Background(), "hello")
	require.NoError(t, err)
}

func TestOpenAIEmbedder_DefaultBaseURL(t *testing.T) {
	// When baseURL is empty, should default to https://api.openai.com/v1/
	e, err := NewOpenAIEmbedder(newTestLLMProvider(), "", "test-key", "text-embedding-3-small")
	require.NoError(t, err)
	assert.Equal(t, "https://api.openai.com/v1/", e.(*openAIEmbedder).baseURL)
}