package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
	"github.com/google/uuid"
)

type noopSessionStore struct{}

var _ storage.SessionStore = (*noopSessionStore)(nil)

func (noopSessionStore) Create(_ context.Context, s *storage.Session) (*storage.Session, error) {
	return s, nil
}
func (noopSessionStore) Get(_ context.Context, _ uuid.UUID) (*storage.Session, error) {
	return nil, fmt.Errorf("session not found")
}
func (noopSessionStore) List(_ context.Context, _, _ int) ([]*storage.Session, int64, error) {
	return nil, 0, nil
}
func (noopSessionStore) Delete(_ context.Context, _ uuid.UUID) error               { return nil }
func (noopSessionStore) UpdateTitle(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (noopSessionStore) UpdateMetadata(_ context.Context, _ uuid.UUID, _ map[string]any) error {
	return nil
}
func (noopSessionStore) GetMessageCount(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}

type noopMessageStore struct{}

var _ storage.MessageStore = (*noopMessageStore)(nil)

func (noopMessageStore) List(_ context.Context, _ uuid.UUID, _ int) ([]*storage.Message, error) {
	return nil, nil
}
func (noopMessageStore) Add(_ context.Context, _ *storage.Message) error    { return nil }
func (noopMessageStore) Update(_ context.Context, _ *storage.Message) error { return nil }
func (noopMessageStore) Upsert(_ context.Context, _ *storage.Message) error { return nil }
func (noopMessageStore) DeleteBySession(_ context.Context, _ uuid.UUID) error {
	return nil
}

type noopTodoStore struct{}

var _ storage.TodoStore = (*noopTodoStore)(nil)

func (noopTodoStore) Create(_ context.Context, t *storage.Todo) (*storage.Todo, error) {
	return t, nil
}
func (noopTodoStore) Get(_ context.Context, _ uuid.UUID) (*storage.Todo, error) {
	return nil, fmt.Errorf("todo not found")
}
func (noopTodoStore) List(_ context.Context, _ uuid.UUID) ([]*storage.Todo, error) {
	return nil, nil
}
func (noopTodoStore) UpdateStatus(_ context.Context, _ uuid.UUID, _ storage.TodoStatus) (*storage.Todo, error) {
	return nil, fmt.Errorf("todo not found")
}
func (noopTodoStore) DeleteBySession(_ context.Context, _ uuid.UUID) error { return nil }

type noopMemoryStore struct{}

var _ storage.MemoryStore = (*noopMemoryStore)(nil)

func (noopMemoryStore) Store(_ context.Context, _ *storage.Memory) error { return nil }
func (noopMemoryStore) Search(_ context.Context, _ []float32, _ int) ([]*storage.Memory, error) {
	return nil, nil
}
func (noopMemoryStore) GetBySession(_ context.Context, _ string, _ int) ([]*storage.Memory, error) {
	return nil, nil
}
func (noopMemoryStore) DeleteBySession(_ context.Context, _ string) error { return nil }

type noopSessionManager struct{}

var _ iface.SessionManager = (*noopSessionManager)(nil)

func (noopSessionManager) GetSession(_ iface.ChatContextInterface) (*storage.Session, error) {
	return nil, fmt.Errorf("session not found")
}
func (noopSessionManager) CreateSession(_ iface.ChatContextInterface, _, _ string, _ ...iface.SessionCreateOption) (*storage.Session, error) {
	return nil, fmt.Errorf("not implemented")
}
func (noopSessionManager) AddAsyncCompletionPending(_ iface.ChatContextInterface, _ map[string]any) error {
	return nil
}

type noopContextManager struct{}

var _ iface.ContextManager = (*noopContextManager)(nil)

func (noopContextManager) AddMessage(_ iface.ChatContextInterface, _ *storage.Message) error {
	return nil
}
func (noopContextManager) UpdateMessage(_ iface.ChatContextInterface, _ *storage.Message) error {
	return nil
}
func (noopContextManager) UpsertMessage(_ iface.ChatContextInterface, _ *storage.Message) error {
	return nil
}
func (noopContextManager) GetHistory(_ iface.ChatContextInterface, _ int) ([]*storage.Message, error) {
	return nil, nil
}
func (noopContextManager) BuildContext(_ iface.ChatContextInterface, _ string, _ int, _ string) ([]entity.MessageForLLM, error) {
	return nil, nil
}

// AgentQuickConfig is a simplified configuration for single-agent use cases.
type AgentQuickConfig struct {
	Name         string
	Model        string
	SystemPrompt string
	Tools        []string // capability names, supports wildcards like "tools.*"
	Hooks        []string // capability names
	LLM          llm.LLMProvider
	SessionStore storage.SessionStore
	MessageStore storage.MessageStore
}

// NewAgent creates a single-agent Harness from an AgentQuickConfig,
// calls Build(), and returns the Engine and Registry.
func NewAgent(cfg AgentQuickConfig) (agent.AgentEngine, agent.AgentRegistry, error) {
	harnessCfg := HarnessConfig{
		Store: StoreConfig{
			Session: cfg.SessionStore,
			Message: cfg.MessageStore,
		},
		LLM:    cfg.LLM,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Agents: []AgentSpec{
			{
				ID:            "default",
				Name:          cfg.Name,
				Model:         cfg.Model,
				SystemPrompt:  cfg.SystemPrompt,
				Tools:         cfg.Tools,
				Hooks:         cfg.Hooks,
				AllowDelegate: false,
			},
		},
	}

	h := NewHarness(harnessCfg)
	if err := h.Build(); err != nil {
		return nil, nil, fmt.Errorf("harness build: %w", err)
	}

	return h.Engine(), h.Registry(), nil
}

type StoreConfig struct {
	Session storage.SessionStore
	Message storage.MessageStore
	Todo    storage.TodoStore
	Memory  storage.MemoryStore
}

// AgentSpec declares a static agent to be auto-converted to an AgentFactory during Build().
type AgentSpec struct {
	ID             string
	Name           string
	Model          string
	SystemPrompt   string
	Tools          []string // capability names, supports wildcards
	Hooks          []string // capability names, supports wildcards
	AllowDelegate  bool
	ToolsDependsOn []string // extra capability dependencies
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
	SessionManager iface.SessionManager // if set, overrides noop
	ContextManager iface.ContextManager // if set, overrides noop
	AsyncTracker   tool.AsyncToolTracker // if set, overrides auto-created
}

type Harness struct {
	config       HarnessConfig
	engine       agent.AgentEngine
	registry     agent.AgentRegistry
	asyncTracker tool.AsyncToolTracker
	built        bool
}

func NewHarness(cfg HarnessConfig) *Harness {
	return &Harness{config: cfg}
}

func (h *Harness) Engine() agent.AgentEngine       { return h.engine }
func (h *Harness) Registry() agent.AgentRegistry    { return h.registry }
func (h *Harness) AsyncTracker() tool.AsyncToolTracker { return h.asyncTracker }

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
// 10. Return Harness with engine + registry references
func (h *Harness) Build() error {
	if h.built {
		return fmt.Errorf("harness already built")
	}

	logger := h.config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	h.initStores()

	allCapabilityNames := h.collectCapabilityNames()
	resolved, err := capabilities.ResolveDependencies(allCapabilityNames)
	if err != nil {
		return fmt.Errorf("resolve capabilities: %w", err)
	}

	toolRegistry := tool.NewToolRegistry()
	hookRunner := hook.NewHookRunner()

	capDeps := capabilities.CapabilityDeps{
		SessionStore: h.config.Store.Session,
		MessageStore: h.config.Store.Message,
		TodoStore:    h.config.Store.Todo,
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

	var sessionMgr iface.SessionManager = &noopSessionManager{}
	var contextMgr iface.ContextManager = &noopContextManager{}
	var asyncTracker tool.AsyncToolTracker = tool.NewAsyncToolRegistry()

	if h.config.SessionManager != nil {
		sessionMgr = h.config.SessionManager
	}
	if h.config.ContextManager != nil {
		contextMgr = h.config.ContextManager
	}
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

	h.engine = agent.NewAgentEngine(agentRegistry, sessionMgr, contextMgr, asyncTracker, engineOpts...)

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

func (h *Harness) initStores() {
	if h.config.Store.Session == nil {
		h.config.Store.Session = &noopSessionStore{}
	}
	if h.config.Store.Message == nil {
		h.config.Store.Message = &noopMessageStore{}
	}
	if h.config.Store.Todo == nil {
		h.config.Store.Todo = &noopTodoStore{}
	}
	if h.config.Store.Memory == nil {
		h.config.Store.Memory = &noopMemoryStore{}
	}
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
		for _, t := range spec.Tools {
			add(t)
		}
		for _, hk := range spec.Hooks {
			add(hk)
		}
		for _, dep := range spec.ToolsDependsOn {
			add(dep)
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
		expandedTools := capabilities.ExpandWildcards(spec.Tools)
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
