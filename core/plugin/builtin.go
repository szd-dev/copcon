package plugin

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/capabilities/hooks"
	"github.com/copcon/core/capabilities/tools"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

type toolNameWrapper struct {
	tool.Tool
	newName string
}

func (w *toolNameWrapper) Name() string { return w.newName }

func (w *toolNameWrapper) IsDelegationTool() bool {
	if dt, ok := w.Tool.(tool.DelegationTool); ok {
		return dt.IsDelegationTool()
	}
	return false
}

type hookNameWrapper struct {
	hook.Hook
	newName string
}

func (w *hookNameWrapper) Name() string { return w.newName }

type delegateToToolWrapper struct {
	inner *tools.DelegateToTool
}

func (w *delegateToToolWrapper) Name() string {
	if w.inner == nil {
		return "builtin.tool.delegate_to"
	}
	return w.inner.Name()
}

func (w *delegateToToolWrapper) Description() string {
	if w.inner == nil {
		return "Delegate a task to another agent"
	}
	return w.inner.Description()
}

func (w *delegateToToolWrapper) InputSchema() map[string]any {
	if w.inner == nil {
		return nil
	}
	return w.inner.InputSchema()
}

func (w *delegateToToolWrapper) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	if w.inner == nil {
		return &tool.ToolResult{Success: false, Error: "delegate_to tool not initialized"}, nil
	}
	return w.inner.Execute(chatCtx, args)
}

func (w *delegateToToolWrapper) IsDelegationTool() bool { return true }

type todoToolWrapper struct {
	inner tool.Tool
}

func (w *todoToolWrapper) Name() string                    { return w.inner.Name() }
func (w *todoToolWrapper) Description() string             { return w.inner.Description() }
func (w *todoToolWrapper) InputSchema() map[string]any     { return w.inner.InputSchema() }
func (w *todoToolWrapper) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	return w.inner.Execute(chatCtx, args)
}

type todoInjectionWrapper struct {
	inner hook.Hook
}

func (w *todoInjectionWrapper) Name() string                { return w.inner.Name() }
func (w *todoInjectionWrapper) Points() []hook.HookPoint   { return w.inner.Points() }
func (w *todoInjectionWrapper) Priority() int               { return w.inner.Priority() }
func (w *todoInjectionWrapper) Execute(ctx *hook.HookContext) error {
	return w.inner.Execute(ctx)
}

type readSubSessionToolWrapper struct {
	inner *tools.ReadSubSessionTool
}

func (w *readSubSessionToolWrapper) Name() string {
	if w.inner == nil {
		return "builtin.tool.read_sub_session"
	}
	return w.inner.Name()
}

func (w *readSubSessionToolWrapper) Description() string {
	if w.inner == nil {
		return "Read messages from a sub-session created by delegate_to"
	}
	return w.inner.Description()
}

func (w *readSubSessionToolWrapper) InputSchema() map[string]any {
	if w.inner == nil {
		return nil
	}
	return w.inner.InputSchema()
}

func (w *readSubSessionToolWrapper) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	if w.inner == nil {
		return &tool.ToolResult{Success: false, Error: "read_sub_session tool not initialized"}, nil
	}
	return w.inner.Execute(chatCtx, args)
}

type todoManagerAdapter struct {
	store storage.TodoStore
}

func (a *todoManagerAdapter) CreateTodo(chatCtx iface.ChatContextInterface, content string, opts ...tool.TodoOption) (*storage.Todo, error) {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}
	todo := &storage.Todo{
		SessionID: sessionUUID,
		Content:   content,
		Status:    storage.TodoStatusPending,
	}
	for _, opt := range opts {
		opt(todo)
	}
	return a.store.Create(chatCtx.Context(), todo)
}

func (a *todoManagerAdapter) GetTodo(chatCtx iface.ChatContextInterface, id string) (*storage.Todo, error) {
	todoID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid todo ID: %w", err)
	}
	return a.store.Get(chatCtx.Context(), todoID)
}

func (a *todoManagerAdapter) ListTodos(chatCtx iface.ChatContextInterface) ([]*storage.Todo, error) {
	sessionUUID, err := uuid.Parse(chatCtx.SessionID())
	if err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}
	return a.store.List(chatCtx.Context(), sessionUUID)
}

func (a *todoManagerAdapter) Delete(chatCtx iface.ChatContextInterface, id string) error {
	return fmt.Errorf("delete by ID not supported via TodoStore adapter")
}

func (a *todoManagerAdapter) Start(chatCtx iface.ChatContextInterface, id string) (*storage.Todo, error) {
	todoID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid todo ID: %w", err)
	}
	return a.store.UpdateStatus(chatCtx.Context(), todoID, storage.TodoStatusInProgress)
}

func (a *todoManagerAdapter) Complete(chatCtx iface.ChatContextInterface, id string, result string) (*storage.Todo, error) {
	todoID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid todo ID: %w", err)
	}
	return a.store.UpdateStatus(chatCtx.Context(), todoID, storage.TodoStatusCompleted)
}

func (a *todoManagerAdapter) Fail(chatCtx iface.ChatContextInterface, id string, reason string) (*storage.Todo, error) {
	todoID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid todo ID: %w", err)
	}
	return a.store.UpdateStatus(chatCtx.Context(), todoID, storage.TodoStatusFailed)
}

type builtin struct {
	delegateTool  *delegateToToolWrapper
	todoTool      *todoToolWrapper
	todoInjection *todoInjectionWrapper
	readSubTool   *readSubSessionToolWrapper
	loggingCfg    hooks.LoggingPluginConfig
}

func NewBuiltin(loggingCfg hooks.LoggingPluginConfig) Plugin {
	b := &builtin{
		delegateTool:  &delegateToToolWrapper{inner: nil},
		todoTool:      &todoToolWrapper{inner: newPlaceholderTodoTool()},
		todoInjection: &todoInjectionWrapper{inner: newPlaceholderTodoInjectionHook()},
		readSubTool:   &readSubSessionToolWrapper{inner: nil},
		loggingCfg:    loggingCfg,
	}
	return b
}

func (b *builtin) Name() string { return "builtin" }

func (b *builtin) Tools() []tool.Tool {
	return []tool.Tool{
		&toolNameWrapper{Tool: tools.NewCodeExecutor(), newName: "builtin.tool.code_executor"},
		&toolNameWrapper{Tool: tools.NewShellExecutor(), newName: "builtin.tool.shell_executor"},
		&toolNameWrapper{Tool: tools.NewFileOps(""), newName: "builtin.tool.file_ops"},
		&toolNameWrapper{Tool: b.todoTool, newName: "builtin.tool.todolist"},
		&toolNameWrapper{Tool: tools.NewConfirmActionTool(), newName: "builtin.tool.confirm_action"},
		&toolNameWrapper{Tool: tools.NewAskUserTool(), newName: "builtin.tool.ask_user"},
		&toolNameWrapper{Tool: tools.NewGetToolStatusTool(tool.NewAsyncToolRegistry()), newName: "builtin.tool.get_tool_status"},
		&toolNameWrapper{Tool: b.delegateTool, newName: "builtin.tool.delegate_to"},
		&toolNameWrapper{Tool: b.readSubTool, newName: "builtin.tool.read_sub_session"},
	}
}

func (b *builtin) Hooks() []hook.Hook {
	return []hook.Hook{
		&hookNameWrapper{Hook: hooks.NewLoggingPlugin(b.loggingCfg), newName: "builtin.hook.logging"},
		&hookNameWrapper{Hook: b.todoInjection, newName: "builtin.hook.todo_injection"},
		&hookNameWrapper{Hook: hooks.NewTracingPlugin(nil), newName: "builtin.hook.tracing"},
	}
}

func (b *builtin) Init(deps PluginDeps) error {
	engine, ok := deps.Engine.(agent.AgentEngine)
	if !ok {
		return fmt.Errorf("builtin.Init: Engine dependency not available or wrong type")
	}

	b.delegateTool.inner = tools.NewDelegateToTool(
		deps.AgentRegistry,
		deps.SessionStore,
		deps.MessageStore,
		engine,
	)

	if deps.TodoStore != nil {
		todoMgr := &todoManagerAdapter{store: deps.TodoStore}
		b.todoTool.inner = tools.NewTodoTool(todoMgr)
		b.todoInjection.inner = hooks.NewTodoInjectionHook(deps.TodoStore)
	}

	if deps.SessionStore != nil && deps.MessageStore != nil {
		b.readSubTool.inner = tools.NewReadSubSessionTool(deps.SessionStore, deps.MessageStore)
	}

	return nil
}

type placeholderTodoTool struct{}

func newPlaceholderTodoTool() tool.Tool { return &placeholderTodoTool{} }

func (p *placeholderTodoTool) Name() string                { return "todolist" }
func (p *placeholderTodoTool) Description() string         { return "Manage todo items (not yet initialized)" }
func (p *placeholderTodoTool) InputSchema() map[string]any { return nil }
func (p *placeholderTodoTool) Execute(_ iface.ChatContextInterface, _ map[string]any) (*tool.ToolResult, error) {
	return &tool.ToolResult{Success: false, Error: "todolist tool not initialized"}, nil
}

type placeholderTodoInjectionHook struct{}

func newPlaceholderTodoInjectionHook() hook.Hook { return &placeholderTodoInjectionHook{} }

func (p *placeholderTodoInjectionHook) Name() string             { return "todo_injection" }
func (p *placeholderTodoInjectionHook) Points() []hook.HookPoint { return nil }
func (p *placeholderTodoInjectionHook) Priority() int            { return 100 }
func (p *placeholderTodoInjectionHook) Execute(_ *hook.HookContext) error { return nil }
