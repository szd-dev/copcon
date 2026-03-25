package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	OpenAI   OpenAIConfig   `yaml:"openai"`
	Qdrant   QdrantConfig   `yaml:"qdrant"`
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

	return &cfg, nil
}
