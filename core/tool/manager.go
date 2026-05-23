package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/llm"
)

var (
	ErrToolNotFound      = errors.New("tool not found")
	ErrToolAlreadyExists = errors.New("tool already exists")
)

type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type ToolResult struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*ToolResult, error)
}

// DelegationTool identifies tools that delegate work to sub-agents.
// Tools implementing this interface are excluded from the automatic
// execution_mode injection in GetToolDefs to avoid schema collisions
// with the tool's own mode parameter.
type DelegationTool interface {
	IsDelegationTool() bool
}

type ToolManager interface {
	Register(tool Tool) error
	Unregister(name string) error
	Get(name string) (Tool, error)
	List() []ToolInfo
	Execute(chatCtx iface.ChatContextInterface, name string, args map[string]any) (*ToolResult, error)
	GetToolDefs() []llm.ToolDef
}

type ToolRegistry interface {
	Register(tool Tool) error
	Get(name string) (Tool, error)
	List() []ToolInfo
}

type toolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewToolRegistry() ToolRegistry {
	return &toolRegistry{
		tools: make(map[string]Tool),
	}
}

func (r *toolRegistry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	r.tools[name] = tool
	return nil
}

func (r *toolRegistry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}

	return tool, nil
}

func (r *toolRegistry) List() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]ToolInfo, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, ToolInfo{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}

	return tools
}

type toolManager struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewToolManager() ToolManager {
	return &toolManager{
		tools: make(map[string]Tool),
	}
}

func (m *toolManager) Register(tool Tool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := tool.Name()
	if _, exists := m.tools[name]; exists {
		return fmt.Errorf("%w: %s", ErrToolAlreadyExists, name)
	}

	m.tools[name] = tool
	return nil
}

func (m *toolManager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tools[name]; !exists {
		return fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}

	delete(m.tools, name)
	return nil
}

func (m *toolManager) Get(name string) (Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tool, exists := m.tools[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}

	return tool, nil
}

func (m *toolManager) List() []ToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make([]ToolInfo, 0, len(m.tools))
	for _, tool := range m.tools {
		tools = append(tools, ToolInfo{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}

	return tools
}

func (m *toolManager) Execute(chatCtx iface.ChatContextInterface, name string, args map[string]any) (*ToolResult, error) {
	tool, err := m.Get(name)
	if err != nil {
		return nil, err
	}

	return tool.Execute(chatCtx, args)
}

func (m *toolManager) GetToolDefs() []llm.ToolDef {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make([]llm.ToolDef, 0, len(m.tools))
	for _, t := range m.tools {
		schema := t.InputSchema()
		if schema == nil {
			schema = make(map[string]any)
		}

		// Create a copy to avoid modifying the original schema
		schemaCopy := make(map[string]any)
		for k, v := range schema {
			schemaCopy[k] = v
		}

		if schemaCopy["properties"] == nil {
			schemaCopy["properties"] = make(map[string]any)
		}
		props := schemaCopy["properties"].(map[string]any)

		// Delegation tools define their own mode parameter; skip auto-injection
		// to avoid schema collisions.
		if _, isDelegation := t.(DelegationTool); !isDelegation {
			props["execution_mode"] = map[string]any{
				"type":        "string",
				"enum":        []string{"sync", "concurrent", "async"},
				"default":     "sync",
				"description": "Execution mode: 'sync' (wait for result), 'concurrent' (parallel with other tools), 'async' (background execution). Default: sync.",
			}
		}

		paramsJSON, _ := json.Marshal(schemaCopy)
		tools = append(tools, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  paramsJSON,
		})
	}

	return tools
}

func ParseArguments(argsJSON string) (map[string]any, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}
	return args, nil
}

func ToArgumentsJSON(args map[string]any) string {
	b, _ := json.Marshal(args)
	return string(b)
}
