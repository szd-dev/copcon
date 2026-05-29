package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/chat"
	"github.com/copcon/core/context_builder"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"

	_ "github.com/copcon/core/capabilities/hooks"
	_ "github.com/copcon/core/capabilities/tools"
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
func (p quickStoreProvider) Knowledge() storage.KnowledgeStore { return nil }

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

var builtInHooks = []string{"hooks.todo_injection", "hooks.memory", "hooks.logging", "hooks.tracing"}

var builtInTools = []string{"tools.confirm_action", "tools.ask_user", "tools.todo", "tools.async"}

var toolNameToCap = map[string]string{
	"code_executor":  "tools.code_executor",
	"shell_executor": "tools.shell_executor",
	"file_ops":       "tools.file_ops",
	"todolist":       "tools.todo",
}

type StoreConfig struct {
	Provider storage.StoreProvider
	Memory   storage.MemoryStore
}

// AgentSpec declares a static agent to be auto-converted to an AgentFactory during Build().
type AgentSpec struct {
	ID            string
	Name          string
	Model         string
	SystemPrompt  string
	Tools         []string
	AllowDelegate bool
	Memory        MemorySpec
	KnowledgeBases []string
}

// MemorySpec defines file-based memory configuration for an agent.
// This is the core-internal equivalent of the server's MemoryConfig.
type MemorySpec struct {
	Enabled       bool
	BasePath      string
	SystemDir     string
	IndexFile     string
	MaxIndexLines int
	MaxIndexBytes int
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
}

type Harness struct {
	config       HarnessConfig
	engine       agent.AgentEngine
	registry     agent.AgentRegistry
	asyncTracker tool.AsyncToolTracker
	sessionStore chat.ActiveSessions
	built        bool
}

func NewHarness(cfg HarnessConfig) *Harness {
	return &Harness{config: cfg}
}

func (h *Harness) Engine() agent.AgentEngine           { return h.engine }
func (h *Harness) Registry() agent.AgentRegistry       { return h.registry }
func (h *Harness) AsyncTracker() tool.AsyncToolTracker { return h.asyncTracker }
func (h *Harness) Store() storage.StoreProvider        { return h.config.Store.Provider }
func (h *Harness) ActiveSessions() chat.ActiveSessions { return h.sessionStore }

// Build executes the full construction sequence:
//  1. Initialize store pointers (nil → no-op)
//  2. Resolve capabilities: expand wildcards + dependency sort
//  3. Create global ToolRegistry + register tools from resolved capabilities
//  4. Create global HookRunner + register hooks from resolved capabilities
//  5. Create AgentRegistry
//  6. Register AgentSpecs as factories (auto-convert)
//  7. Register AgentFactorySpecs as factories (direct)
//  8. Create AgentEngine with registry + session/message stores
//  9. Register cross-agent tools (delegate_to, read_sub_session)
//
// 10. Return Harness with engine + registry references
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

	allCapabilityNames := h.collectCapabilityNames()
	resolved, err := capabilities.ResolveDependencies(allCapabilityNames)
	if err != nil {
		return fmt.Errorf("resolve capabilities: %w", err)
	}

	toolRegistry := tool.NewToolRegistry()
	hookRunner := hook.NewHookRunner()

	capDeps := capabilities.CapabilityDeps{
		SessionStore: h.config.Store.Provider.Sessions(),
		MessageStore: h.config.Store.Provider.Messages(),
		TodoStore:    h.config.Store.Provider.Todos(),
		MemoryStore:  h.config.Store.Memory,
		Logger:       logger,
	}

	capToToolName := make(map[string]string)

	for _, cap := range resolved {
		switch cap.Type() {
		case capabilities.CapabilityTypeTool:
			if cap.Name() == "tools.delegate" || cap.Name() == "tools.read_sub_session" {
				continue
			}
			tc, ok := cap.(capabilities.ToolCapability)
			if !ok {
				return fmt.Errorf("capability %q has type tool but does not implement ToolCapability", cap.Name())
			}
			t, err := tc.NewTool(capDeps)
			if err != nil {
				return fmt.Errorf("create tool from capability %q: %w", cap.Name(), err)
			}
			if err := toolRegistry.Register(t); err != nil {
				return fmt.Errorf("register tool %q: %w", cap.Name(), err)
			}
			capToToolName[cap.Name()] = t.Name()
			logger.Info("harness: registered tool", "capability", cap.Name(), "tool", t.Name())

		case capabilities.CapabilityTypeHook:
			if cap.Name() == "hooks.memory" && h.config.Store.Memory == nil {
				logger.Info("harness: skipping memory hook (MemoryStore not configured)", "capability", cap.Name())
				continue
			}
			hc, ok := cap.(capabilities.HookCapability)
			if !ok {
				return fmt.Errorf("capability %q has type hook but does not implement HookCapability", cap.Name())
			}
			hk, err := hc.NewHook(capDeps)
			if err != nil {
				return fmt.Errorf("create hook from capability %q: %w", cap.Name(), err)
			}
			hookRunner.Register(hk)
			logger.Info("harness: registered hook", "capability", cap.Name(), "hook", hk.Name())
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
			h.makeAgentFactory(spec, toolRegistry, hookRunner, logger, capToToolName),
		)
		logger.Info("harness: registered agent spec", "id", spec.ID, "name", spec.Name, "model", spec.Model)
	}

	for _, spec := range h.config.AgentFactories {
		agentRegistry.RegisterFactory(spec.ID, spec.Name, spec.Model, spec.AllowDelegate, spec.Factory)
		logger.Info("harness: registered agent factory", "id", spec.ID, "name", spec.Name, "model", spec.Model)
	}

	var asyncTracker tool.AsyncToolTracker = tool.NewAsyncToolRegistry()
	if h.config.AsyncTracker != nil {
		asyncTracker = h.config.AsyncTracker
	}

	engineOpts := []agent.EngineOption{
		agent.WithHookRunner(hookRunner),
		agent.WithLogger(logger),
	}
	if h.config.LLM != nil {
		engineOpts = append(engineOpts, agent.WithLLMProvider(h.config.LLM))
	}

	ctxBuilder := context_builder.New()

	h.engine = agent.NewAgentEngine(agentRegistry, h.config.Store.Provider.Sessions(), h.config.Store.Provider.Messages(), ctxBuilder, asyncTracker, engineOpts...)

	h.asyncTracker = asyncTracker

	capDeps.AgentRegistry = agentRegistry
	capDeps.Engine = h.engine

	registerCrossAgentTool("tools.delegate", capDeps, toolRegistry, logger, capToToolName)
	registerCrossAgentTool("tools.read_sub_session", capDeps, toolRegistry, logger, capToToolName)

	h.registry = agentRegistry
	h.built = true

	logger.Info("harness: build complete",
		"agents", len(agentRegistry.List()),
		"tools", len(toolRegistry.List()),
	)

	return nil
}

func registerCrossAgentTool(capName string, capDeps capabilities.CapabilityDeps, toolRegistry tool.ToolRegistry, logger *slog.Logger, capToToolName map[string]string) {
	cap, ok := capabilities.Get(capName)
	if !ok {
		return
	}
	tc, ok := cap.(capabilities.ToolCapability)
	if !ok {
		return
	}
	t, err := tc.NewTool(capDeps)
	if err != nil {
		logger.Warn("harness: failed to create cross-agent tool", "capability", capName, "error", err)
		return
	}
	if err := toolRegistry.Register(t); err != nil {
		logger.Warn("harness: failed to register cross-agent tool", "capability", capName, "error", err)
		return
	}
	capToToolName[capName] = t.Name()
	logger.Info("harness: registered cross-agent tool", "tool", t.Name())
}

func (h *Harness) initStores() error {
	if h.config.Store.Provider == nil {
		return fmt.Errorf("StoreConfig.Provider is required")
	}
	return nil
}

func (h *Harness) collectCapabilityNames() []string {
	seen := make(map[string]bool)
	var names []string

	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}

	for _, spec := range h.config.Agents {
		for _, t := range builtInTools {
			add(t)
		}
		for _, hk := range builtInHooks {
			add(hk)
		}
		for _, t := range spec.Tools {
			if capName, ok := toolNameToCap[t]; ok {
				add(capName)
			} else {
				add(t)
			}
		}
	}

	return names
}

func (h *Harness) makeAgentFactory(
	spec AgentSpec,
	toolRegistry tool.ToolRegistry,
	_ hook.HookRunner,
	_ *slog.Logger,
	capToToolName map[string]string,
) agent.AgentFactory {
	return func(_ context.Context, params agent.CreateParams) (agent.AgentDefinition, error) {
		toolMgr := tool.NewToolManager()

		capNames := make([]string, 0, len(builtInTools)+len(spec.Tools))
		capNames = append(capNames, builtInTools...)
		for _, t := range spec.Tools {
			if capName, ok := toolNameToCap[t]; ok {
				capNames = append(capNames, capName)
			} else {
				capNames = append(capNames, t)
			}
		}

		expandedTools := capabilities.ExpandWildcards(capNames)
		registeredNames := make(map[string]bool)

		for _, capName := range expandedTools {
			toolName, mapped := capToToolName[capName]
			if !mapped {
				toolName = capName
			}
			if registeredNames[toolName] {
				continue
			}
			if t, err := toolRegistry.Get(toolName); err == nil {
				_ = toolMgr.Register(t)
				registeredNames[t.Name()] = true
			}
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
