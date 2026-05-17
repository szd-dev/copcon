package agent

import (
	"errors"
	"fmt"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/llm"
	"github.com/copcon/server/internal/tool"
)

var (
	ErrAgentNotFound  = errors.New("agent not found")
	ErrNoDefaultAgent = errors.New("no default agent configured")
)

type AgentDefinition struct {
	ID           string
	Name         string
	Model        string
	SystemPrompt string
	ToolManager  tool.ToolManager
	LLMProvider  llm.LLMProvider
}

type AgentInfo struct {
	ID    string
	Name  string
	Model string
}

type AgentRegistry interface {
	Get(id string) (AgentDefinition, error)
	List() []AgentInfo
	Default() (AgentDefinition, error)
}

type agentRegistry struct {
	mu           sync.RWMutex
	agents       map[string]AgentDefinition
	defaultAgent string
}

func NewAgentRegistry(cfg *config.Config, toolRegistry tool.ToolRegistry) (AgentRegistry, error) {
	registry := &agentRegistry{
		agents:       make(map[string]AgentDefinition),
		defaultAgent: cfg.DefaultAgentID,
	}

	for _, agentConfig := range cfg.Agents {
		// Validate that all tools exist
		for _, toolName := range agentConfig.Tools {
			if _, err := toolRegistry.Get(toolName); err != nil {
				return nil, fmt.Errorf("agent %s: tool not found: %s", agentConfig.ID, toolName)
			}
		}

		// Create tool manager with only the tools specified for this agent
		toolMgr := tool.NewToolManager()
		for _, toolName := range agentConfig.Tools {
			t, err := toolRegistry.Get(toolName)
			if err != nil {
				return nil, fmt.Errorf("agent %s: failed to get tool %s: %w", agentConfig.ID, toolName, err)
			}
			if err := toolMgr.Register(t); err != nil {
				return nil, fmt.Errorf("agent %s: failed to register tool %s: %w", agentConfig.ID, toolName, err)
			}
		}

		// Create OpenAI client
		opts := []option.RequestOption{
			option.WithAPIKey(cfg.OpenAI.APIKey),
		}
		baseURL := agentConfig.BaseURL
		if baseURL == "" {
			baseURL = cfg.OpenAI.BaseURL
		}
		if baseURL != "" {
			opts = append(opts, option.WithBaseURL(baseURL))
		}
		client := openai.NewClient(opts...)
		provider := llm.NewOpenAIAdapter(&client, agentConfig.Model)

		// Create agent definition
		agent := AgentDefinition{
			ID:           agentConfig.ID,
			Name:         agentConfig.Name,
			Model:        agentConfig.Model,
			SystemPrompt: agentConfig.SystemPrompt,
			ToolManager:  toolMgr,
			LLMProvider:  provider,
		}

		registry.agents[agentConfig.ID] = agent
	}

	return registry, nil
}

func (r *agentRegistry) Get(id string) (AgentDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[id]
	if !exists {
		return AgentDefinition{}, ErrAgentNotFound
	}

	return agent, nil
}

func (r *agentRegistry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]AgentInfo, 0, len(r.agents))
	for _, agent := range r.agents {
		infos = append(infos, AgentInfo{
			ID:    agent.ID,
			Name:  agent.Name,
			Model: agent.Model,
		})
	}

	return infos
}

func (r *agentRegistry) Default() (AgentDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaultAgent == "" {
		return AgentDefinition{}, ErrNoDefaultAgent
	}

	agent, exists := r.agents[r.defaultAgent]
	if !exists {
		return AgentDefinition{}, ErrNoDefaultAgent
	}

	return agent, nil
}
