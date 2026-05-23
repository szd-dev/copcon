package tools

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

type TodoTool struct {
	todoMgr TodoManager
}

func NewTodoTool(todoMgr TodoManager) *TodoTool {
	return &TodoTool{todoMgr: todoMgr}
}

func (t *TodoTool) Name() string {
	return "todolist"
}

func (t *TodoTool) Description() string {
	return "Manage todo items for a session. Actions: create, start, complete, fail, list, replan"
}

func (t *TodoTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "start", "complete", "fail", "list", "replan"},
				"description": "Action to perform on todos",
			},
			"todo_id": map[string]any{
				"type":        "string",
				"description": "Todo ID (required for start, complete, fail actions)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Todo content (required for create action)",
			},
			"validation": map[string]any{
				"type":        "string",
				"description": "Validation rules or failure reason (for create, fail actions)",
			},
			"depends_on": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "List of todo IDs this todo depends on (for create action)",
			},
			"result": map[string]any{
				"type":        "string",
				"description": "Result of completing the todo (required for complete action)",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Reason for failure (for fail action)",
			},
			"todos": map[string]any{
				"type":        "array",
				"description": "List of todos to replace existing ones (for replan action)",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{
							"type":        "string",
							"description": "Todo content",
						},
						"validation": map[string]any{
							"type":        "string",
							"description": "Validation rules",
						},
						"depends_on": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "List of todo IDs this todo depends on",
						},
					},
					"required": []string{"content"},
				},
			},
		},
		"required": []string{"action"},
	}
}

func (t *TodoTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	action, ok := args["action"].(string)
	if !ok {
		return errorResult("action is required")
	}

	switch action {
	case "create":
		return t.handleCreate(chatCtx, args)
	case "start":
		return t.handleStart(chatCtx, args)
	case "complete":
		return t.handleComplete(chatCtx, args)
	case "fail":
		return t.handleFail(chatCtx, args)
	case "list":
		return t.handleList(chatCtx)
	case "replan":
		return t.handleReplan(chatCtx, args)
	default:
		return errorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *TodoTool) handleCreate(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	content, ok := args["content"].(string)
	if !ok || content == "" {
		return errorResult("content is required for create action")
	}

	var opts []TodoOption

	if validation, ok := args["validation"].(string); ok && validation != "" {
		opts = append(opts, WithValidation(validation))
	}

	if depsRaw, ok := args["depends_on"].([]any); ok && len(depsRaw) > 0 {
		deps := make([]string, 0, len(depsRaw))
		for _, d := range depsRaw {
			if depStr, ok := d.(string); ok {
				deps = append(deps, depStr)
			}
		}
		if len(deps) > 0 {
			opts = append(opts, WithDependsOn(deps...))
		}
	}

	todoItem, err := t.todoMgr.CreateTodo(chatCtx, content, opts...)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to create todo during replan: %v", err))
	}

	return successResult(todoToMap(todoItem))
}

func (t *TodoTool) handleStart(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	todoID, ok := args["todo_id"].(string)
	if !ok || todoID == "" {
		return errorResult("todo_id is required for start action")
	}

	todoItem, err := t.todoMgr.Start(chatCtx, todoID)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to start todo: %v", err))
	}

	return successResult(todoToMap(todoItem))
}

func (t *TodoTool) handleComplete(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	todoID, ok := args["todo_id"].(string)
	if !ok || todoID == "" {
		return errorResult("todo_id is required for complete action")
	}

	result, ok := args["result"].(string)
	if !ok || result == "" {
		return errorResult("result is required for complete action")
	}

	todoItem, err := t.todoMgr.Complete(chatCtx, todoID, result)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to complete todo: %v", err))
	}

	return successResult(todoToMap(todoItem))
}

func (t *TodoTool) handleFail(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	todoID, ok := args["todo_id"].(string)
	if !ok || todoID == "" {
		return errorResult("todo_id is required for fail action")
	}

	reason, _ := args["reason"].(string)
	if reason == "" {
		reason, _ = args["validation"].(string)
	}

	todoItem, err := t.todoMgr.Fail(chatCtx, todoID, reason)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to mark todo as failed: %v", err))
	}

	return successResult(todoToMap(todoItem))
}

func (t *TodoTool) handleList(chatCtx iface.ChatContextInterface) (*tool.ToolResult, error) {
	todos, err := t.todoMgr.ListTodos(chatCtx)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to list todos: %v", err))
	}

	todoList := make([]map[string]any, 0, len(todos))
	for _, todoItem := range todos {
		todoList = append(todoList, todoToMap(todoItem))
	}

	return successResult(map[string]any{
		"todos": todoList,
		"count": len(todoList),
	})
}

func (t *TodoTool) handleReplan(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	todosRaw, ok := args["todos"].([]any)
	if !ok {
		return errorResult("todos array is required for replan action")
	}

	existingTodos, err := t.todoMgr.ListTodos(chatCtx)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to list existing todos: %v", err))
	}

	for _, existing := range existingTodos {
		if err := t.todoMgr.Delete(chatCtx, existing.ID.String()); err != nil {
			return errorResult(fmt.Sprintf("failed to delete existing todo: %v", err))
		}
	}

	createdTodos := make([]map[string]any, 0, len(todosRaw))
	for _, todoRaw := range todosRaw {
		todoMap, ok := todoRaw.(map[string]any)
		if !ok {
			continue
		}

		content, ok := todoMap["content"].(string)
		if !ok || content == "" {
			continue
		}

		var opts []TodoOption

		if validation, ok := todoMap["validation"].(string); ok && validation != "" {
			opts = append(opts, WithValidation(validation))
		}

		if depsRaw, ok := todoMap["depends_on"].([]any); ok && len(depsRaw) > 0 {
			deps := make([]string, 0, len(depsRaw))
			for _, d := range depsRaw {
				if depStr, ok := d.(string); ok {
					deps = append(deps, depStr)
				}
			}
			if len(deps) > 0 {
				opts = append(opts, WithDependsOn(deps...))
			}
		}

		todoItem, err := t.todoMgr.CreateTodo(chatCtx, content, opts...)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create todo during replan: %v", err))
		}

		createdTodos = append(createdTodos, todoToMap(todoItem))
	}

	return successResult(map[string]any{
		"todos":   createdTodos,
		"count":   len(createdTodos),
		"message": "Todos replanned successfully",
	})
}

func todoToMap(todoItem *storage.Todo) map[string]any {
	dependsOn := make([]string, 0, len(todoItem.DependsOn))
	for _, dep := range todoItem.DependsOn {
		dependsOn = append(dependsOn, dep.String())
	}

	result := map[string]any{
		"id":          todoItem.ID.String(),
		"session_id":  todoItem.SessionID.String(),
		"content":     todoItem.Content,
		"status":      todoItem.Status,
		"created_at":  todoItem.CreatedAt,
		"updated_at":  todoItem.UpdatedAt,
		"retry_count": todoItem.RetryCount,
	}

	if todoItem.ActiveForm != "" {
		result["active_form"] = todoItem.ActiveForm
	}
	if todoItem.Validation != "" {
		result["validation"] = todoItem.Validation
	}
	if todoItem.Result != "" {
		result["result"] = todoItem.Result
	}
	if len(dependsOn) > 0 {
		result["depends_on"] = dependsOn
	}
	if todoItem.CompletedAt != nil {
		result["completed_at"] = *todoItem.CompletedAt
	}

	return result
}

var _ tool.Tool = (*TodoTool)(nil)

func init() {
	capabilities.Register(&todoCapability{})
}

type todoCapability struct{}

func (c *todoCapability) Name() string                         { return "tools.todo" }
func (c *todoCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeTool }
func (c *todoCapability) DependsOn() []string                  { return []string{"hooks.todo_injection"} }
func (c *todoCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return NewTodoTool(newTodoManagerFromDeps(deps)), nil
}

// newTodoManagerFromDeps wraps CapabilityDeps.TodoStore as a TodoManager.
func newTodoManagerFromDeps(deps capabilities.CapabilityDeps) TodoManager {
	return &todoManagerAdapter{store: deps.TodoStore}
}

type todoManagerAdapter struct {
	store storage.TodoStore
}

func (a *todoManagerAdapter) CreateTodo(chatCtx iface.ChatContextInterface, content string, opts ...TodoOption) (*storage.Todo, error) {
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