package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigWithAgents(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	configContent := `
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "test"

openai:
  api_key: "test-key"
  base_url: "https://api.test.com/v1"
  model: "gpt-4"

qdrant:
  host: "localhost"
  port: 6333

agents:
  - id: "agent-1"
    name: "General Assistant"
    model: "gpt-4"
    system_prompt: "You are a helpful assistant."
    tools:
      - "code"
      - "shell"
    base_url: "https://api.test.com/v1"
  - id: "agent-2"
    name: "Code Expert"
    model: "gpt-4-turbo"
    system_prompt: "You are a coding expert."
    tools:
      - "code"
    base_url: ""

default_agent_id: "agent-1"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set CONFIG_PATH env var
	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	// Load config
	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify agents loaded correctly
	assert.Len(t, cfg.Agents, 2)
	assert.Equal(t, "agent-1", cfg.DefaultAgentID)

	// Verify first agent
	assert.Equal(t, "agent-1", cfg.Agents[0].ID)
	assert.Equal(t, "General Assistant", cfg.Agents[0].Name)
	assert.Equal(t, "gpt-4", cfg.Agents[0].Model)
	assert.Equal(t, "You are a helpful assistant.", cfg.Agents[0].SystemPrompt)
	assert.Equal(t, []string{"code", "shell"}, cfg.Agents[0].Tools)
	assert.Equal(t, "https://api.test.com/v1", cfg.Agents[0].BaseURL)

	// Verify second agent
	assert.Equal(t, "agent-2", cfg.Agents[1].ID)
	assert.Equal(t, "Code Expert", cfg.Agents[1].Name)
	assert.Equal(t, "gpt-4-turbo", cfg.Agents[1].Model)
	assert.Equal(t, []string{"code"}, cfg.Agents[1].Tools)
}

func TestGetAgent(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{ID: "agent-1", Name: "Agent One", Model: "gpt-4"},
			{ID: "agent-2", Name: "Agent Two", Model: "gpt-3.5"},
		},
	}

	// Test getting existing agent
	agent, err := cfg.GetAgent("agent-1")
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agent.ID)
	assert.Equal(t, "Agent One", agent.Name)

	// Test getting another existing agent
	agent2, err := cfg.GetAgent("agent-2")
	require.NoError(t, err)
	assert.Equal(t, "agent-2", agent2.ID)

	// Test getting non-existent agent
	_, err = cfg.GetAgent("agent-3")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestValidateAgentIDs(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	// Test duplicate agent IDs
	configContent := `
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "test"

openai:
  api_key: "test-key"
  base_url: "https://api.test.com/v1"
  model: "gpt-4"

qdrant:
  host: "localhost"
  port: 6333

agents:
  - id: "agent-1"
    name: "Agent One"
    model: "gpt-4"
  - id: "agent-1"
    name: "Agent One Duplicate"
    model: "gpt-4"

default_agent_id: "agent-1"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	_, err = Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestValidateDefaultAgentID(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	// Test non-existent default agent ID
	configContent := `
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "test"

openai:
  api_key: "test-key"
  base_url: "https://api.test.com/v1"
  model: "gpt-4"

qdrant:
  host: "localhost"
  port: 6333

agents:
  - id: "agent-1"
    name: "Agent One"
    model: "gpt-4"

default_agent_id: "non-existent-agent"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	_, err = Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "default agent ID")
}

func TestLoadConfigWithoutAgents(t *testing.T) {
	// Test backward compatibility - config without agents should still work
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	configContent := `
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "test"

openai:
  api_key: "test-key"
  base_url: "https://api.test.com/v1"
  model: "gpt-4"

qdrant:
  host: "localhost"
  port: 6333
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Agents should be empty but not nil
	assert.Empty(t, cfg.Agents)
	assert.Empty(t, cfg.DefaultAgentID)
}

func TestValidateAmbiguousDatabaseConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	configContent := `
server:
  port: "8080"

database:
  host: "localhost"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "test"
  sqlite_path: "data/copcon.db"

openai:
  api_key: "test-key"
  base_url: "https://api.test.com/v1"
  model: "gpt-4"

qdrant:
  host: "localhost"
  port: 6333
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	_, err = Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous database config")
}

func TestValidatePostgresMissingHost(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	configContent := `
server:
  port: "8080"

database:
  type: "postgres"
  port: 5432
  user: "admin"
  password: "changeme"
  dbname: "test"

openai:
  api_key: "test-key"
  base_url: "https://api.test.com/v1"
  model: "gpt-4"

qdrant:
  host: "localhost"
  port: 6333
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	_, err = Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is not configured")
}

func TestValidateKnowledgeEmbeddingBackendUnsupported(t *testing.T) {
	cfg := &Config{
		Knowledge: KnowledgeConfig{
			Embedding: KnowledgeEmbedConfig{Backend: "bge_m3"},
		},
	}

	err := cfg.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "knowledge embedding backend")
}

func TestValidateKnowledgeEmbeddingBackendEmpty(t *testing.T) {
	cfg := &Config{
		Knowledge: KnowledgeConfig{
			SQLitePath: "data/knowledge.db",
		},
	}

	err := cfg.validate()
	assert.NoError(t, err)
}

func TestValidateMemoryBasePathNotAbsolute(t *testing.T) {
	cfg := &Config{
		Memory: MemoryConfig{
			Enabled:  true,
			BasePath: "relative/path",
		},
	}

	err := cfg.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "memory.base_path must be an absolute path")
}

func TestValidateMemoryBasePathTildePrefix(t *testing.T) {
	cfg := &Config{
		Memory: MemoryConfig{
			Enabled:  true,
			BasePath: "~/.copcon/memory",
		},
	}

	err := cfg.validate()
	assert.NoError(t, err)
}

func TestValidateMemoryBasePathAbsolute(t *testing.T) {
	cfg := &Config{
		Memory: MemoryConfig{
			Enabled:  true,
			BasePath: "/data/copcon/memory",
		},
	}

	err := cfg.validate()
	assert.NoError(t, err)
}

func TestValidateEmptyMemoryBasePathIsValid(t *testing.T) {
	cfg := &Config{
		Memory: MemoryConfig{
			Enabled: true,
		},
	}

	err := cfg.validate()
	assert.NoError(t, err)
}
