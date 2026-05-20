package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/llm"
	"github.com/copcon/server/internal/tool"
)

var (
	ErrAgentNotFound  = errors.New("agent not found")
	ErrNoDefaultAgent = errors.New("no default agent configured")
)

// CreateParams holds parameters for creating an agent instance at runtime.
// These override or extend the agent's base configuration.
type CreateParams struct {
	Task          string         // task description injected into SystemPrompt
	ParentContext string         // parent agent's session context
	ModelOverride string         // override the default model
	Extra         map[string]any // arbitrary extra parameters
}

// AgentFactory is a function that creates an AgentDefinition from a set of
// runtime parameters. Factories are registered once and invoked on demand
// when an agent instance is needed (e.g. via Get).
type AgentFactory func(ctx context.Context, params CreateParams) (AgentDefinition, error)

// factoryEntry stores metadata and the factory function for a registered agent.
type factoryEntry struct {
	name          string
	model         string
	allowDelegate bool
	factory       AgentFactory
}

type AgentDefinition struct {
	ID           string
	Name         string
	Model        string
	SystemPrompt string
	ToolManager  tool.ToolManager
	LLMProvider  llm.LLMProvider
	Hooks        []hook.Hook
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

	// RegisterFactory registers a factory for creating agent instances on demand.
	// allowDelegate controls whether this agent appears in ListDelegatable output.
	RegisterFactory(id, name, model string, allowDelegate bool, factory AgentFactory)

	// GetFactory returns the registered factory for the given agent ID.
	GetFactory(id string) (AgentFactory, error)

	// ListDelegatable returns only agents that have allowDelegate set to true.
	ListDelegatable() []AgentInfo
}

type agentRegistry struct {
	mu           sync.RWMutex
	factories    map[string]factoryEntry
	defaultAgent string
}

func NewAgentRegistry(cfg *config.Config, toolRegistry tool.ToolRegistry) (AgentRegistry, error) {
	registry := &agentRegistry{
		factories:    make(map[string]factoryEntry),
		defaultAgent: cfg.DefaultAgentID,
	}

	for _, agentConfig := range cfg.Agents {
		for _, toolName := range agentConfig.Tools {
			if _, err := toolRegistry.Get(toolName); err != nil {
				return nil, fmt.Errorf("agent %s: tool not found: %s", agentConfig.ID, toolName)
			}
		}

		ac := agentConfig

		factory := func(ctx context.Context, params CreateParams) (AgentDefinition, error) {
			model := ac.Model
			if params.ModelOverride != "" {
				model = params.ModelOverride
			}

			toolMgr := tool.NewToolManager()
			for _, toolName := range ac.Tools {
				t, err := toolRegistry.Get(toolName)
				if err != nil {
					return AgentDefinition{}, fmt.Errorf("agent %s: failed to get tool %s: %w", ac.ID, toolName, err)
				}
				if err := toolMgr.Register(t); err != nil {
					return AgentDefinition{}, fmt.Errorf("agent %s: failed to register tool %s: %w", ac.ID, toolName, err)
				}
			}

			opts := []option.RequestOption{
				option.WithAPIKey(cfg.OpenAI.APIKey),
			}
			baseURL := ac.BaseURL
			if baseURL == "" {
				baseURL = cfg.OpenAI.BaseURL
			}
			if baseURL != "" {
				opts = append(opts, option.WithBaseURL(baseURL))
			}
			client := openai.NewClient(opts...)
			provider := llm.NewOpenAIAdapter(&client, model)

			systemPrompt := ac.SystemPrompt
			if params.Task != "" {
				systemPrompt = systemPrompt + "\n\nCurrent Task: " + params.Task
			}

			return AgentDefinition{
				ID:           ac.ID,
				Name:         ac.Name,
				Model:        model,
				SystemPrompt: systemPrompt,
				ToolManager:  toolMgr,
				LLMProvider:  provider,
			}, nil
		}

		registry.RegisterFactory(agentConfig.ID, agentConfig.Name, agentConfig.Model, false, factory)
	}

	return registry, nil
}

func (r *agentRegistry) RegisterFactory(id, name, model string, allowDelegate bool, factory AgentFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[id] = factoryEntry{
		name:          name,
		model:         model,
		allowDelegate: allowDelegate,
		factory:       factory,
	}
}

func (r *agentRegistry) GetFactory(id string) (AgentFactory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.factories[id]
	if !exists {
		return nil, ErrAgentNotFound
	}

	return entry.factory, nil
}

func (r *agentRegistry) ListDelegatable() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]AgentInfo, 0, len(r.factories))
	for id, entry := range r.factories {
		if entry.allowDelegate {
			infos = append(infos, AgentInfo{
				ID:    id,
				Name:  entry.name,
				Model: entry.model,
			})
		}
	}

	return infos
}

func (r *agentRegistry) Get(id string) (AgentDefinition, error) {
	factory, err := r.GetFactory(id)
	if err != nil {
		return AgentDefinition{}, err
	}

	return factory(context.Background(), CreateParams{})
}

func (r *agentRegistry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]AgentInfo, 0, len(r.factories))
	for id, entry := range r.factories {
		infos = append(infos, AgentInfo{
			ID:    id,
			Name:  entry.name,
			Model: entry.model,
		})
	}

	return infos
}

func (r *agentRegistry) Default() (AgentDefinition, error) {
	r.mu.RLock()
	defaultID := r.defaultAgent
	r.mu.RUnlock()

	if defaultID == "" {
		return AgentDefinition{}, ErrNoDefaultAgent
	}

	factory, err := r.GetFactory(defaultID)
	if err != nil {
		return AgentDefinition{}, ErrNoDefaultAgent
	}

	return factory(context.Background(), CreateParams{})
}
