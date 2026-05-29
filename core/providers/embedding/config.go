package embedding

// BackendType identifies the embedding provider backend.
type BackendType string

const (
	// BackendOpenAI uses OpenAI's embedding API (e.g. text-embedding-3-small).
	BackendOpenAI BackendType = "openai"

	// BackendBGEM3 uses BGE-M3 via a local endpoint. Reserved for future use.
	BackendBGEM3 BackendType = "bge_m3"
)

// EmbeddingConfig configures the embedding backend.
type EmbeddingConfig struct {
	// Backend selects the embedding provider.
	Backend BackendType `yaml:"backend"`

	// OpenAIModel is the model identifier when Backend is BackendOpenAI.
	// Example: "text-embedding-3-small".
	OpenAIModel string `yaml:"openai_model"`

	// BaseURL is the OpenAI API base URL (e.g. "https://api.openai.com/v1/").
	// Only used when Backend is BackendOpenAI.
	BaseURL string `yaml:"base_url"`

	// APIKey is the OpenAI API key.
	// Only used when Backend is BackendOpenAI.
	APIKey string `yaml:"api_key"`

	// BGEM3Endpoint is the HTTP endpoint for BGE-M3 when Backend is BackendBGEM3.
	// Reserved for future use.
	BGEM3Endpoint string `yaml:"bge_m3_endpoint"`
}