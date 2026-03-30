package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	Server         ServerConfig   `yaml:"server"`
	Database       DatabaseConfig `yaml:"database"`
	OpenAI         OpenAIConfig   `yaml:"openai"`
	Qdrant         QdrantConfig   `yaml:"qdrant"`
	Agents         []AgentConfig  `yaml:"agents"`
	DefaultAgentID string         `yaml:"default_agent_id"`
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
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
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
	idSet := make(map[string]bool)
	for _, agent := range c.Agents {
		if idSet[agent.ID] {
			return fmt.Errorf("duplicate agent ID: %s", agent.ID)
		}
		idSet[agent.ID] = true
	}

	if c.DefaultAgentID != "" && !idSet[c.DefaultAgentID] {
		return fmt.Errorf("default agent ID not found: %s", c.DefaultAgentID)
	}

	return nil
}

func (c *Config) GetAgent(id string) (AgentConfig, error) {
	for _, agent := range c.Agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return AgentConfig{}, fmt.Errorf("agent with ID %s not found", id)
}
