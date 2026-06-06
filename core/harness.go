package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/chat"
	"github.com/copcon/core/context_builder"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

// AgentQuickConfig is a simplified configuration for single-agent use cases.
type AgentQuickConfig struct {
	Name         string
	Model        string
	SystemPrompt string
	Tools        []string
	Hooks        []string
	LLM          llm.LLMProvider
	SessionStore storage.SessionStore
	MessageStore storage.MessageStore
}

type quickStoreProvider struct {
	sessionStore storage.SessionStore
	messageStore storage.MessageStore
}

func (p quickStoreProvider) Sessions() storage.SessionStore   { return p.sessionStore }
func (p quickStoreProvider) Messages() storage.MessageStore   { return p.messageStore }
func (p quickStoreProvider) Todos() storage.TodoStore         { return nil }


// NewAgent creates a single-agent Harness from an AgentQuickConfig,
// calls Build(), and returns the Engine and Registry.
func NewAgent(cfg AgentQuickConfig) (agent.AgentEngine, agent.AgentRegistry, error) {
	harnessCfg := HarnessConfig{
		Store: StoreConfig{
			Provider: quickStoreProvider{cfg.SessionStore, cfg.MessageStore},
		},
		LLM:    cfg.LLM,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Agents: []AgentSpec{
			{ID: "default", Name: cfg.Name, Model: cfg.Model, SystemPrompt: cfg.SystemPrompt, Tools: cfg.Tools, AllowDelegate: false},
		},
	}

	h := NewHarness(harnessCfg)
	if err := h.Build(); err != nil {
		return nil, nil, fmt.Errorf("harness build: %w", err)
	}

	return h.Engine(), h.Registry(), nil
}

type StoreConfig struct {
	Provider storage.StoreProvider
}

// AgentSpec declares a static agent to be auto-converted to an AgentFactory during Build().
type AgentSpec struct {
	ID             string
	Name           string
	Model          string
	SystemPrompt   string
	Tools          []string
	AllowDelegate  bool
	KnowledgeBases []string
}



// AgentFactorySpec declares a dynamic agent factory to be registered directly.
type AgentFactorySpec struct {
	ID            string
	Name          string
	Model         string
	Factory       agent.AgentFactory
	AllowDelegate bool
}

type HarnessConfig struct {
	Store          StoreConfig
	LLM            llm.LLMProvider
	Logger         *slog.Logger
	Agents         []AgentSpec
	AgentFactories []AgentFactorySpec
	AsyncTracker   tool.AsyncToolTracker
	Plugins        []plugin.Plugin
}

type Harness struct {
	config       HarnessConfig
	engine       agent.AgentEngine
	registry     agent.AgentRegistry
	asyncTracker tool.AsyncToolTracker
	sessionStore chat.ActiveSessions
	hookRunner   hook.HookRunner
	built        bool
	plugins      []plugin.Plugin
	toolPool     *plugin.ToolPool
	hookPool     *plugin.HookPool
}

func NewHarness(cfg HarnessConfig) *Harness {
	return &Harness{
		config:  cfg,
		plugins: cfg.Plugins,
	}
}

func (h *Harness) Register(p plugin.Plugin) {
	h.plugins = append(h.plugins, p)
}

func (h *Harness) Engine() agent.AgentEngine           { return h.engine }
func (h *Harness) Registry() agent.AgentRegistry       { return h.registry }
func (h *Harness) AsyncTracker() tool.AsyncToolTracker { return h.asyncTracker }
func (h *Harness) Store() storage.StoreProvider        { return h.config.Store.Provider }
func (h *Harness) ActiveSessions() chat.ActiveSessions { return h.sessionStore }
func (h *Harness) HookRunner() hook.HookRunner         { return h.hookRunner }
func (h *Harness) ToolPool() *plugin.ToolPool          { return h.toolPool }
func (h *Harness) HookPool() *plugin.HookPool          { return h.hookPool }

// Build executes the full construction sequence using the plugin system.
func (h *Harness) Build() error {
	if h.built {
		return fmt.Errorf("harness already built")
	}

	h.sessionStore = chat.NewActiveSessions()

	logger := h.config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	if err := h.initStores(); err != nil {
		return err
	}

	h.toolPool = plugin.NewToolPool()
	h.hookPool = plugin.NewHookPool()

	for _, p := range h.plugins {
		for _, t := range p.Tools() {
			h.toolPool.Register(t)
			logger.Info("harness: registered plugin tool", "plugin", p.Name(), "tool", t.Name())
		}
		for _, hk := range p.Hooks() {
			h.hookPool.Register(hk)
			logger.Info("harness: registered plugin hook", "plugin", p.Name(), "hook", hk.Name())
		}
	}

	agentKBs := make(map[string][]string)
	for _, spec := range h.config.Agents {
		if len(spec.KnowledgeBases) > 0 {
			agentKBs[spec.ID] = spec.KnowledgeBases
		}
	}

	defaultAgentID := ""
	if len(h.config.Agents) > 0 {
		defaultAgentID = h.config.Agents[0].ID
	} else if len(h.config.AgentFactories) > 0 {
		defaultAgentID = h.config.AgentFactories[0].ID
	}
	agentRegistry := agent.NewAgentRegistry(defaultAgentID)

	for i := range h.config.Agents {
		spec := h.config.Agents[i]
		agentRegistry.RegisterFactory(spec.ID, spec.Name, spec.Model, spec.AllowDelegate,
			h.makePluginAgentFactory(spec),
		)
		logger.Info("harness: registered agent spec", "id", spec.ID, "name", spec.Name, "model", spec.Model)
	}

	for _, spec := range h.config.AgentFactories {
		agentRegistry.RegisterFactory(spec.ID, spec.Name, spec.Model, spec.AllowDelegate, spec.Factory)
		logger.Info("harness: registered agent factory", "id", spec.ID, "name", spec.Name, "model", spec.Model)
	}

	h.hookRunner = hook.NewHookRunner()
	for _, hk := range h.hookPool.All() {
		h.hookRunner.Register(hk)
	}

	var asyncTracker tool.AsyncToolTracker = tool.NewAsyncToolRegistry()
	if h.config.AsyncTracker != nil {
		asyncTracker = h.config.AsyncTracker
	}

	engineOpts := []agent.EngineOption{
		agent.WithHookRunner(h.hookRunner),
		agent.WithLogger(logger),
	}
	if h.config.LLM != nil {
		engineOpts = append(engineOpts, agent.WithLLMProvider(h.config.LLM))
	}

	ctxBuilder := context_builder.New()

	h.engine = agent.NewAgentEngine(agentRegistry, h.config.Store.Provider.Sessions(), h.config.Store.Provider.Messages(), ctxBuilder, asyncTracker, engineOpts...)
	h.asyncTracker = asyncTracker

	for _, p := range h.plugins {
		deps := plugin.PluginDeps{
			SessionStore:        h.config.Store.Provider.Sessions(),
			MessageStore:        h.config.Store.Provider.Messages(),
			TodoStore:           h.config.Store.Provider.Todos(),
			AgentRegistry:       agentRegistry,
			Engine:              h.engine,
			Logger:              logger,
			AgentKnowledgeBases: agentKBs,
		}
		if err := p.Init(deps); err != nil {
			return fmt.Errorf("init plugin %s: %w", p.Name(), err)
		}
		logger.Info("harness: initialized plugin", "plugin", p.Name())
	}

	h.registry = agentRegistry
	h.built = true

	logger.Info("harness: build complete",
		"agents", len(agentRegistry.List()),
		"tools", len(h.toolPool.Select([]string{"*"})),
	)

	return nil
}

func (h *Harness) initStores() error {
	if h.config.Store.Provider == nil {
		return fmt.Errorf("StoreConfig.Provider is required")
	}
	return nil
}

func (h *Harness) makePluginAgentFactory(spec AgentSpec) agent.AgentFactory {
	return func(_ context.Context, params agent.CreateParams) (agent.AgentDefinition, error) {
		toolMgr := tool.NewToolManager()

		selectedTools := h.toolPool.Select(spec.Tools)
		for _, t := range selectedTools {
			_ = toolMgr.Register(t)
		}

		model := spec.Model
		if params.ModelOverride != "" {
			model = params.ModelOverride
		}

		systemPrompt := spec.SystemPrompt
		if params.Task != "" {
			systemPrompt = systemPrompt + "\n\nCurrent Task: " + params.Task
		}

		return agent.AgentDefinition{
			ID:           spec.ID,
			Name:         spec.Name,
			Model:        model,
			SystemPrompt: systemPrompt,
			ToolManager:  toolMgr,
			LLMProvider:  h.config.LLM,
		}, nil
	}
}
