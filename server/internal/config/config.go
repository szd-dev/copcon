package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	Server         ServerConfig    `yaml:"server"`
	Database       DatabaseConfig  `yaml:"database"`
	OpenAI         OpenAIConfig    `yaml:"openai"`
	Qdrant         QdrantConfig    `yaml:"qdrant"`
	Agents         []AgentConfig   `yaml:"agents"`
	DefaultAgentID string          `yaml:"default_agent_id"`
	Knowledge      KnowledgeConfig `yaml:"knowledge,omitempty"`
	Memory         MemoryConfig    `yaml:"memory,omitempty"`
	Skills         SkillConfig     `yaml:"skills,omitempty"`
}

type AgentConfig struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Model        string   `yaml:"model"`
	SystemPrompt string   `yaml:"system_prompt"`
	Tools        []string `yaml:"tools"`
	BaseURL      string   `yaml:"base_url"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
}

type DatabaseConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	User       string `yaml:"user"`
	Password   string `yaml:"password"`
	DBName     string `yaml:"dbname"`
	Type       string `yaml:"type"`        // "postgres" | "sqlite" | "" (auto-detect)
	SQLitePath string `yaml:"sqlite_path"` // SQLite file path, default "data/copcon.db"
}

type OpenAIConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

type QdrantConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type MemoryConfig struct {
	Enabled       bool                      `yaml:"enabled"`
	BasePath      string                    `yaml:"base_path,omitempty"`
	SystemDir     string                    `yaml:"system_dir,omitempty"`
	IndexFile     string                    `yaml:"index_file,omitempty"`
	MaxIndexLines int                       `yaml:"max_index_lines,omitempty"`
	MaxIndexBytes int                       `yaml:"max_index_bytes,omitempty"`
	Summarization MemorySummarizationConfig `yaml:"summarization,omitempty"`
}

type SkillConfig struct {
	Enabled    bool     `yaml:"enabled"`
	ExtraPaths []string `yaml:"extra_paths,omitempty"`
}

type MemorySummarizationConfig struct {
	Enabled     bool                      `yaml:"enabled"`
	Model       string                    `yaml:"model,omitempty"`
	APIKey      string                    `yaml:"api_key,omitempty"`
	BaseURL     string                    `yaml:"base_url,omitempty"`
	MaxTokens   int                       `yaml:"max_tokens,omitempty"`
	Temperature float64                   `yaml:"temperature,omitempty"`
	Trigger     SummarizationTriggerConfig `yaml:"trigger,omitempty"`
}

type SummarizationTriggerConfig struct {
	MaxMemories     int `yaml:"max_memories,omitempty"`     // default: 50
	MaxAgeHours     int `yaml:"max_age_hours,omitempty"`    // default: 24
	CooldownMinutes int `yaml:"cooldown_minutes,omitempty"` // default: 60
}

// KnowledgeConfig configures the knowledge base module.
// KB metadata (ID, name, chunk_size, etc.) is managed via the API, not here.
type KnowledgeConfig struct {
	SQLitePath    string               `yaml:"sqlite_path,omitempty"`    // default: "data/collab/knowledge.db"
	VectorBackend string               `yaml:"vector_backend,omitempty"` // "brute-force" (default) or "sqlite-vec"
	Embedding     KnowledgeEmbedConfig `yaml:"embedding,omitempty"`
}

// KnowledgeEmbedConfig configures the embedding model used by the knowledge module.
// APIKey/BaseURL inherit from OpenAI config when not specified.
type KnowledgeEmbedConfig struct {
	Backend string `yaml:"backend,omitempty"` // "openai" (default if Model is set)
	APIKey  string `yaml:"api_key,omitempty"`
	BaseURL string `yaml:"base_url,omitempty"`
	Model   string `yaml:"model,omitempty"`
}

func Load() (*Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	cfg := Config{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.OpenAI.APIKey = apiKey
	}

	if cfg.Knowledge.Embedding.APIKey == "" {
		cfg.Knowledge.Embedding.APIKey = cfg.OpenAI.APIKey
	}
	if cfg.Knowledge.Embedding.BaseURL == "" {
		cfg.Knowledge.Embedding.BaseURL = cfg.OpenAI.BaseURL
	}
	if cfg.Knowledge.Embedding.Backend == "" && cfg.Knowledge.Embedding.Model != "" {
		cfg.Knowledge.Embedding.Backend = "openai"
	}

	// Inherit summarization config from OpenAI if not set
	if cfg.Memory.Summarization.APIKey == "" {
		cfg.Memory.Summarization.APIKey = cfg.OpenAI.APIKey
	}
	if cfg.Memory.Summarization.BaseURL == "" {
		cfg.Memory.Summarization.BaseURL = cfg.OpenAI.BaseURL
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Database.Host != "" && c.Database.SQLitePath != "" && c.Database.Type == "" {
		return fmt.Errorf("ambiguous database config: both host (%s) and sqlite_path (%s) specified without explicit type", c.Database.Host, c.Database.SQLitePath)
	}
	if c.Database.Type == "postgres" && c.Database.Host == "" {
		return fmt.Errorf("database type is postgres but host is not configured")
	}

	agentIDSet := make(map[string]bool)
	for _, agent := range c.Agents {
		if agentIDSet[agent.ID] {
			return fmt.Errorf("duplicate agent ID: %s", agent.ID)
		}
		agentIDSet[agent.ID] = true
	}

	if c.DefaultAgentID != "" && !agentIDSet[c.DefaultAgentID] {
		return fmt.Errorf("default agent ID not found: %s", c.DefaultAgentID)
	}

	if c.Knowledge.Embedding.Backend != "" && c.Knowledge.Embedding.Backend != "openai" {
		return fmt.Errorf("knowledge embedding backend %q is not supported (only \"openai\" is supported)", c.Knowledge.Embedding.Backend)
	}

	if c.Memory.BasePath != "" {
		if !filepath.IsAbs(c.Memory.BasePath) && !strings.HasPrefix(c.Memory.BasePath, "~/") {
			return fmt.Errorf("memory.base_path must be an absolute path or start with ~/, got %q", c.Memory.BasePath)
		}
	}

	return nil
}

func (d DatabaseConfig) DSN() string {
	return "host=" + d.Host + " port=" + strconv.Itoa(d.Port) + " user=" + d.User + " password=" + d.Password + " dbname=" + d.DBName + " sslmode=disable"
}

func (d DatabaseConfig) HasPostgresConfig() bool {
	return d.Host != ""
}

func (c *Config) GetAgent(id string) (AgentConfig, error) {
	for _, agent := range c.Agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return AgentConfig{}, fmt.Errorf("agent with ID %s not found", id)
}