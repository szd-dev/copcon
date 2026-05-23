package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/tool"
)

var (
	ErrAgentNotFound  = errors.New("agent not found")
	ErrNoDefaultAgent = errors.New("no default agent configured")
)

type CreateParams struct {
	Task          string
	ParentContext string
	ModelOverride string
	Extra         map[string]any
}

type AgentFactory func(ctx context.Context, params CreateParams) (AgentDefinition, error)

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

	RegisterFactory(id, name, model string, allowDelegate bool, factory AgentFactory)
	GetFactory(id string) (AgentFactory, error)
	ListDelegatable() []AgentInfo
}

type agentRegistry struct {
	mu           sync.RWMutex
	factories    map[string]factoryEntry
	defaultAgent string
}

func NewAgentRegistry(defaultAgentID string) AgentRegistry {
	registry := &agentRegistry{
		factories:    make(map[string]factoryEntry),
		defaultAgent: defaultAgentID,
	}

	return registry
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
