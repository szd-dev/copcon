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
	Server         ServerConfig          `yaml:"server"`
	Database       DatabaseConfig        `yaml:"database"`
	OpenAI         OpenAIConfig          `yaml:"openai"`
	Qdrant         QdrantConfig          `yaml:"qdrant"`
	Agents         []AgentConfig         `yaml:"agents"`
	DefaultAgentID string                `yaml:"default_agent_id"`
	KnowledgeBases []KnowledgeBaseConfig `yaml:"knowledge_bases,omitempty"`
}

type AgentConfig struct {
	ID             string          `yaml:"id"`
	Name           string          `yaml:"name"`
	Model          string          `yaml:"model"`
	SystemPrompt   string          `yaml:"system_prompt"`
	Tools          []string        `yaml:"tools"`
	BaseURL        string          `yaml:"base_url"`
	Memory         MemoryConfig    `yaml:"memory,omitempty"`
	KnowledgeBases []string        `yaml:"knowledge_bases,omitempty"`
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
	Enabled       bool   `yaml:"enabled"`
	BasePath      string `yaml:"base_path,omitempty"`
	SystemDir     string `yaml:"system_dir,omitempty"`
	IndexFile     string `yaml:"index_file,omitempty"`
	MaxIndexLines int    `yaml:"max_index_lines,omitempty"`
	MaxIndexBytes int    `yaml:"max_index_bytes,omitempty"`
}

type EmbeddingConfig struct {
	Backend       string `yaml:"backend"`
	OpenAIModel   string `yaml:"openai_model,omitempty"`
	BGEM3Endpoint string `yaml:"bge_m3_endpoint,omitempty"`
}

type KnowledgeBaseConfig struct {
	ID           string          `yaml:"id"`
	Name         string          `yaml:"name"`
	Backend      string          `yaml:"backend"`
	SQLitePath   string          `yaml:"sqlite_path,omitempty"`
	ChunkSize    int             `yaml:"chunk_size,omitempty"`
	ChunkOverlap int             `yaml:"chunk_overlap,omitempty"`
	Embedding    EmbeddingConfig `yaml:"embedding"`
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

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Override with env vars if set
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.OpenAI.APIKey = apiKey
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	// Validate database config: ambiguous if both PG and SQLite paths specified without explicit type
	if c.Database.Host != "" && c.Database.SQLitePath != "" && c.Database.Type == "" {
		return fmt.Errorf("ambiguous database config: both host (%s) and sqlite_path (%s) specified without explicit type", c.Database.Host, c.Database.SQLitePath)
	}
	// Validate: explicit postgres type but missing host
	if c.Database.Type == "postgres" && c.Database.Host == "" {
		return fmt.Errorf("database type is postgres but host is not configured")
	}

	// Validate agent IDs are unique
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

	// Validate knowledge base IDs are unique
	kbIDSet := make(map[string]int)
	for i, kb := range c.KnowledgeBases {
		if _, exists := kbIDSet[kb.ID]; exists {
			return fmt.Errorf("duplicate knowledge base ID: %s", kb.ID)
		}
		kbIDSet[kb.ID] = i
	}

	// Validate agent knowledge base references
	if len(kbIDSet) > 0 || len(c.Agents) > 0 {
		for _, agent := range c.Agents {
			for _, kbRef := range agent.KnowledgeBases {
				if _, exists := kbIDSet[kbRef]; !exists {
					return fmt.Errorf("agent %q references unknown knowledge base %q", agent.ID, kbRef)
				}
				// Validate embedding backend is supported
				kb := c.KnowledgeBases[kbIDSet[kbRef]]
				if kb.Embedding.Backend != "" && kb.Embedding.Backend != "openai" {
					return fmt.Errorf("knowledge base %q has unsupported embedding backend %q (only \"openai\" is supported)", kb.ID, kb.Embedding.Backend)
				}
			}
		}
	}

	// Validate agent memory base path
	for _, agent := range c.Agents {
		if agent.Memory.BasePath != "" {
			if !filepath.IsAbs(agent.Memory.BasePath) && !strings.HasPrefix(agent.Memory.BasePath, "~/") {
				return fmt.Errorf("agent %q memory.base_path must be an absolute path or start with ~/, got %q", agent.ID, agent.Memory.BasePath)
			}
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
