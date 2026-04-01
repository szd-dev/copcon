# Agent Todo List Capability

## TL;DR

> **Quick Summary**: Add task management capability to AI Agent with todo lists scoped to sessions. Agent can plan, execute, and track tasks through a 5-state lifecycle with validation and evidence requirements.
> 
> **Deliverables**:
> - Backend: Todo data model + TodoManager + MCP Tool + REST API + SSE events
> - Frontend: TodoList + TodoItem components + Sidebar integration
> 
> **Estimated Effort**: Medium
> **Parallel Execution**: YES - 3 waves (backend foundation → backend features → frontend)
> **Critical Path**: Database Schema → TodoManager → MCP Tool → Agent Integration → Frontend

---

## Context

### Original Request
为 Agent 增加 Todo list 能力，封装在 session 实体中。

### Interview Summary

**Key Discussions**:
- **Task Planning Mode**: Hybrid (static planning + dynamic replan) - Agent plans upfront, user confirms, execution with replan support
- **User Intervention**: Confirmation-based - User approves plan before execution, execution is autonomous
- **Subtask Support**: Flat structure (MVP) - No nesting, all todos at same level
- **Frontend Display**: Independent sidebar panel - Separate from main chat area
- **Test Strategy**: Tests after implementation
- **Replan Triggers**: Agent self-judgment OR user explicit request
- **Retry Strategy**: Max 3 retries with simple retry
- **Evidence Format**: Plain text description
- **DependsOn Implementation**: JSONB array

**Research Findings**:
- **Current Architecture**: Session has JSONB Metadata (unused), Messages with CASCADE delete pattern, AutoMigrate for schema
- **Industry Best Practices**: 5-state machine (pending/in_progress/completed/blocked/failed), validation mandatory, evidence required (Anthropic, BabyAGI, CrewAI)
- **Production Patterns**: CrewAI Task model with dependencies, LangGraph StateGraph, AutoGen TaskRunner

### Metis Review

**Identified Gaps** (addressed):
- **Evidence format**: Resolved → Plain text description
- **DependsOn implementation**: Resolved → JSONB array
- **CASCADE behavior**: Confirmed → Todos CASCADE delete with Session (follows Message pattern)
- **Circular dependency detection**: Added to validation logic
- **State transition matrix**: Defined explicitly in plan
- **Concurrency handling**: Last-write-wins for MVP (simple approach)

---

## Work Objectives

### Core Objective
Add todo list capability to AI Agent, enabling task planning, execution tracking, and status management scoped to individual sessions.

### Concrete Deliverables
- **Backend**:
  - `todos` table with Session FK
  - TodoManager interface and implementation
  - `manage_todo` MCP Tool
  - REST API endpoints (GET, POST, PATCH, DELETE)
  - SSE events (todo_created, todo_status_changed, todo_completed, todo_deleted)
- **Frontend**:
  - TodoList component (displays list)
  - TodoItem component (individual todo with status icon)
  - Sidebar integration in demo app

### Definition of Done
- [ ] Backend: `go test ./internal/todo/... -v` passes all tests
- [ ] Backend: `go test ./internal/tools/todo_tool_test.go -v` passes
- [ ] Frontend: Todo sidebar renders and updates via SSE
- [ ] Integration: LLM can create/complete todos via tool in real chat session

### Must Have
- Todo data model with 5-state lifecycle
- TodoManager with CRUD + state transitions
- MCP Tool for LLM integration
- REST API endpoints
- SSE real-time updates
- Frontend sidebar component

### Must NOT Have (Guardrails)
- NO subtask nesting (flat list only)
- NO todo history/versioning
- NO cross-session todos
- NO priority levels (MVP)
- NO templates or presets
- NO bulk operations
- NO time tracking
- NO storing todos in Session.Metadata (use proper table)
- NO direct DB access from handlers (must use TodoManager)
- NO `any` type in TypeScript (strict mode)

---

## Verification Strategy

### Test Decision
- **Infrastructure exists**: YES (PostgreSQL + testify)
- **Automated tests**: YES (Tests after implementation)
- **Framework**: Go testing + testify/assert
- **Coverage target**: 80%+ for TodoManager, 100% for state transitions

### QA Policy
Every task includes agent-executed QA scenarios:
- **Backend**: `go test` commands with specific assertions
- **Frontend**: Playwright browser automation (playwright skill)
- **Integration**: Real LLM interaction via API

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Backend Foundation — can start immediately):
├── Task 1: Database schema + migration [quick]
├── Task 2: Todo model definition [quick]
├── Task 3: TodoManager interface [quick]
├── Task 4: TodoManager CRUD implementation [quick]
└── Task 5: State machine validation logic [quick]

Wave 2 (Backend Features — after Wave 1):
├── Task 6: MCP Tool implementation [quick]
├── Task 7: REST API endpoints [quick]
├── Task 8: SSE event integration [quick]
├── Task 9: Tool registration in Agent Engine [quick]
└── Task 10: Backend tests [unspecified-high]

Wave 3 (Frontend — after Wave 2):
├── Task 11: TodoItem component [visual-engineering]
├── Task 12: TodoList component [visual-engineering]
├── Task 13: Sidebar layout integration [visual-engineering]
├── Task 14: SSE event subscription [visual-engineering]
└── Task 15: Frontend tests [visual-engineering]

Wave FINAL (Verification — after ALL tasks):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Integration test with real LLM (unspecified-high)
└── Task F4: Scope fidelity check (deep)

Critical Path: Task 1 → Task 4 → Task 6 → Task 9 → Task 11 → Task 13 → F1-F4
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 5 (Wave 1)
```

### Dependency Matrix

- **1-5**: — — 6-10, 1
- **6**: 1, 4 — 9, 1
- **7**: 3, 4 — 8, 1
- **8**: 7 — 14, 1
- **9**: 6 — F3, 1
- **10**: 4, 6, 7, 8 — — 1
- **11-15**: 8, 9 — F1-F4, 1
- **F1-F4**: 9, 15 — — 1

---

## TODOs

- [ ] 1. Database Schema + Migration

  **What to do**:
  - Create `todos` table in PostgreSQL with proper constraints
  - Add `todos` to AutoMigrate in `cmd/server/main.go`
  - Follow existing Message model pattern exactly (UUID PK, timestamps, FK with CASCADE)
  
  **Schema**:
  ```sql
  CREATE TABLE todos (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
      content TEXT NOT NULL,
      active_form VARCHAR(255),
      status VARCHAR(20) NOT NULL DEFAULT 'pending',
      depends_on UUID[] DEFAULT '{}',
      validation TEXT,
      result TEXT,
      retry_count INTEGER DEFAULT 0,
      created_at TIMESTAMP NOT NULL DEFAULT NOW(),
      updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
      completed_at TIMESTAMP
  );
  CREATE INDEX idx_todos_session_id ON todos(session_id);
  CREATE INDEX idx_todos_status ON todos(status);
  ```

  **Must NOT do**:
  - Do NOT use custom ENUM type (use VARCHAR for simplicity)
  - Do NOT store todos in Session.Metadata
  - Do NOT skip the CASCADE constraint

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Single-file changes following established patterns
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3, 4, 5)
  - **Blocks**: Tasks 6-10 (backend features)
  - **Blocked By**: None

  **References**:
  - `server/internal/session/model.go:85-93` - Session struct pattern (UUID PK, timestamps, relationships)
  - `server/internal/session/model.go:95-104` - Message struct pattern (FK with CASCADE)
  - `server/cmd/server/main.go:33` - AutoMigrate pattern
  - `server/internal/session/model.go:13-36` - JSONB custom type for arrays

  **Acceptance Criteria**:
  - [ ] `todos` table exists in database with all columns
  - [ ] Foreign key constraint `session_id REFERENCES sessions(id) ON DELETE CASCADE` exists
  - [ ] Indexes on `session_id` and `status` created
  - [ ] AutoMigrate includes `&session.Todo{}`

  **QA Scenarios**:
  ```
  Scenario: Table creation verification
    Tool: Bash (psql)
    Steps:
      1. psql -d agent_infra -c "\d todos"
    Expected Result: Table exists with columns: id, session_id, content, status, depends_on, validation, result, retry_count, created_at, updated_at, completed_at
    Evidence: .sisyphus/evidence/task-01-table-schema.txt

  Scenario: CASCADE delete works
    Tool: Bash (psql)
    Steps:
      1. INSERT INTO sessions (id) VALUES ('test-uuid-1');
      2. INSERT INTO todos (id, session_id, content) VALUES ('todo-uuid-1', 'test-uuid-1', 'Test');
      3. DELETE FROM sessions WHERE id = 'test-uuid-1';
      4. SELECT * FROM todos WHERE session_id = 'test-uuid-1';
    Expected Result: Query returns 0 rows (todo was cascade deleted)
    Evidence: .sisyphus/evidence/task-01-cascade-delete.txt
  ```

  **Commit**: YES
  - Message: `feat(todo): add database schema and Todo model`
  - Files: `server/internal/session/model.go`, `server/cmd/server/main.go`

- [ ] 2. Todo Model Definition

  **What to do**:
  - Define `Todo` struct in `server/internal/session/todo.go`
  - Define `TodoStatus` constants (pending, in_progress, completed, blocked, failed)
  - Implement `TableName()` method for GORM
  - Add `Todos []Todo` relationship to Session struct

  **Model**:
  ```go
  type TodoStatus string

  const (
      TodoStatusPending    TodoStatus = "pending"
      TodoStatusInProgress TodoStatus = "in_progress"
      TodoStatusCompleted  TodoStatus = "completed"
      TodoStatusBlocked    TodoStatus = "blocked"
      TodoStatusFailed     TodoStatus = "failed"
  )

  type Todo struct {
      ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
      SessionID   uuid.UUID      `gorm:"type:uuid;not null;index" json:"session_id"`
      Content     string         `gorm:"not null" json:"content"`
      ActiveForm  string         `gorm:"size:255" json:"active_form,omitempty"`
      Status      TodoStatus     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
      DependsOn   pq.StringArray `gorm:"type:uuid[];default:'{}'" json:"depends_on,omitempty"`
      Validation  string         `gorm:"type:text" json:"validation,omitempty"`
      Result      string         `gorm:"type:text" json:"result,omitempty"`
      RetryCount  int            `gorm:"default:0" json:"retry_count"`
      CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
      UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
      CompletedAt *time.Time     `json:"completed_at,omitempty"`
      
      Session *Session `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
  }

  func (Todo) TableName() string {
      return "todos"
  }
  ```

  **Must NOT do**:
  - Do NOT add fields beyond spec (priority, tags, etc.)
  - Do NOT use iota for status (use string constants)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Single struct definition following existing patterns
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3, 4, 5)
  - **Blocks**: Tasks 3, 4 (need Todo type)
  - **Blocked By**: None

  **References**:
  - `server/internal/session/model.go:85-93` - Session struct with UUID and timestamps
  - `server/internal/session/model.go:95-104` - Message struct with FK relationship
  - `server/internal/session/model.go:13-36` - JSONB/custom types for arrays

  **Acceptance Criteria**:
  - [ ] Todo struct defined with all required fields
  - [ ] TodoStatus constants defined (5 values)
  - [ ] GORM tags match schema from Task 1
  - [ ] Session struct has `Todos []Todo` relationship

  **QA Scenarios**:
  ```
  Scenario: Struct compiles and GORM tags are valid
    Tool: Bash (go build)
    Steps:
      1. cd server && go build ./internal/session/...
    Expected Result: Build succeeds without errors
    Evidence: .sisyphus/evidence/task-02-build.txt

  Scenario: Status constants match schema values
    Tool: Bash (go test)
    Steps:
      1. Create test verifying TodoStatusPending == "pending", etc.
      2. go test ./internal/session/... -run TestTodoStatus -v
    Expected Result: All 5 status constants match expected strings
    Evidence: .sisyphus/evidence/task-02-status-constants.txt
  ```

  **Commit**: NO (groups with Task 1)

- [ ] 3. TodoManager Interface

  **What to do**:
  - Define `TodoManager` interface in `server/internal/todo/manager.go`
  - Follow SessionManager pattern exactly (CRUD + session-scoped operations)
  - Define method signatures for state transitions

  **Interface**:
  ```go
  type TodoManager interface {
      // CRUD
      Create(ctx context.Context, sessionID string, content string, opts ...TodoOption) (*session.Todo, error)
      Get(ctx context.Context, id string) (*session.Todo, error)
      List(ctx context.Context, sessionID string) ([]*session.Todo, error)
      Update(ctx context.Context, id string, updates map[string]any) (*session.Todo, error)
      Delete(ctx context.Context, id string) error
      
      // State transitions
      Start(ctx context.Context, id string) (*session.Todo, error)
      Complete(ctx context.Context, id string, result string) (*session.Todo, error)
      Fail(ctx context.Context, id string, reason string) (*session.Todo, error)
      Block(ctx context.Context, id string, reason string) (*session.Todo, error)
      Unblock(ctx context.Context, id string) (*session.Todo, error)
      
      // Utility
      GetAvailableTodos(ctx context.Context, sessionID string) ([]*session.Todo, error)
      GetDB() *gorm.DB
  }
  ```

  **Must NOT do**:
  - Do NOT add methods beyond spec (no Priority, no Tags)
  - Do NOT include implementation (interface only)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Interface definition, no implementation
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 4, 5)
  - **Blocks**: Tasks 4, 7 (need interface)
  - **Blocked By**: Task 2 (needs Todo type)

  **References**:
  - `server/internal/session/manager.go:16-24` - SessionManager interface pattern
  - `server/internal/tool/manager.go:1-15` - Manager interface patterns

  **Acceptance Criteria**:
  - [ ] TodoManager interface defined with all CRUD methods
  - [ ] State transition methods defined (Start, Complete, Fail, Block, Unblock)
  - [ ] GetAvailableTodos method defined (returns non-blocked pending todos)
  - [ ] GetDB method for database access

  **QA Scenarios**:
  ```
  Scenario: Interface compiles
    Tool: Bash (go build)
    Steps:
      1. cd server && go build ./internal/todo/...
    Expected Result: Build succeeds
    Evidence: .sisyphus/evidence/task-03-interface-build.txt
  ```

  **Commit**: NO (groups with Task 4)

- [ ] 4. TodoManager CRUD Implementation

  **What to do**:
  - Implement `todoManager` struct in `server/internal/todo/manager.go`
  - Implement all CRUD methods (Create, Get, List, Update, Delete)
  - Use GORM with context propagation (`db.WithContext(ctx)`)
  - Check `result.RowsAffected` for mutations

  **Implementation Pattern**:
  ```go
  type todoManager struct {
      db *gorm.DB
  }

  func NewTodoManager(db *gorm.DB) TodoManager {
      return &todoManager{db: db}
  }

  func (m *todoManager) Create(ctx context.Context, sessionID string, content string, opts ...TodoOption) (*session.Todo, error) {
      todo := &session.Todo{
          SessionID: uuid.MustParse(sessionID),
          Content:   content,
          Status:    session.TodoStatusPending,
      }
      for _, opt := range opts {
          opt(todo)
      }
      result := m.db.WithContext(ctx).Create(todo)
      if result.Error != nil {
          return nil, fmt.Errorf("create todo: %w", result.Error)
      }
      return todo, nil
  }
  // ... other CRUD methods
  ```

  **Must NOT do**:
  - Do NOT skip context propagation
  - Do NOT ignore GORM errors
  - Do NOT bypass TodoManager in handlers

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Standard CRUD following SessionManager pattern
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 3, 5)
  - **Blocks**: Tasks 6, 7 (need TodoManager)
  - **Blocked By**: Tasks 2, 3 (needs Todo type and interface)

  **References**:
  - `server/internal/session/manager.go:34-49` - Create implementation pattern
  - `server/internal/session/manager.go:51-63` - Get implementation pattern
  - `server/internal/session/manager.go:65-79` - List implementation pattern
  - `server/internal/session/manager.go:81-90` - Delete implementation pattern

  **Acceptance Criteria**:
  - [ ] Create method works with sessionID and content
  - [ ] Get method returns todo by ID
  - [ ] List method returns all todos for session (sorted by created_at)
  - [ ] Update method applies partial updates
  - [ ] Delete method removes todo

  **QA Scenarios**:
  ```
  Scenario: Create and retrieve todo
    Tool: Bash (go test)
    Steps:
      1. Create test with setupTestDB(t) pattern
      2. Create session, then create todo
      3. Retrieve todo by ID
    Expected Result: Todo returned with correct content and session_id
    Evidence: .sisyphus/evidence/task-04-crud-create.txt

  Scenario: List todos by session
    Tool: Bash (go test)
    Steps:
      1. Create session with 3 todos
      2. Call List(sessionID)
    Expected Result: Returns 3 todos, sorted by created_at
    Evidence: .sisyphus/evidence/task-04-crud-list.txt
  ```

  **Commit**: YES
  - Message: `feat(todo): implement TodoManager with CRUD operations`
  - Files: `server/internal/todo/manager.go`
  - Pre-commit: `go test ./internal/todo/... -v`

- [ ] 5. State Machine Validation Logic

  **What to do**:
  - Implement state transition validation in TodoManager
  - Define valid transition matrix
  - Implement dependency checking (DependsOn resolution)
  - Implement circular dependency detection

  **State Transition Matrix**:
  ```
  From            To              Allowed?    Condition
  ────────────────────────────────────────────────────────
  pending    →    in_progress     YES         All dependencies completed
  pending    →    blocked         YES         Dependencies not satisfied
  blocked    →    pending         YES         Dependencies now satisfied
  in_progress →   completed       YES         Result provided
  in_progress →   failed          YES         Error occurred
  completed  →    *               NO          Terminal state
  failed     →    pending         YES         Retry count < 3
  ```

  **Implementation**:
  ```go
  func (m *todoManager) Start(ctx context.Context, id string) (*session.Todo, error) {
      todo, err := m.Get(ctx, id)
      if err != nil {
          return nil, err
      }
      
      // Validate current state
      if todo.Status != session.TodoStatusPending {
          return nil, fmt.Errorf("cannot start todo in status %s", todo.Status)
      }
      
      // Check dependencies
      if err := m.checkDependencies(ctx, todo); err != nil {
          return nil, err
      }
      
      // Update status
      return m.Update(ctx, id, map[string]any{
          "status": session.TodoStatusInProgress,
      })
  }

  func (m *todoManager) checkDependencies(ctx context.Context, todo *session.Todo) error {
      for _, depID := range todo.DependsOn {
          dep, err := m.Get(ctx, depID)
          if err != nil || dep.Status != session.TodoStatusCompleted {
              return fmt.Errorf("dependency %s not satisfied", depID)
          }
      }
      return nil
  }
  ```

  **Must NOT do**:
  - Do NOT allow transitions from terminal states (completed)
  - Do NOT skip dependency validation
  - Do NOT allow circular dependencies (detect and reject)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Logic implementation, single file
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 3, 4)
  - **Blocks**: Tasks 6 (tool needs state transitions)
  - **Blocked By**: Task 4 (needs TodoManager CRUD)

  **References**:
  - Industry best practices from research (5-state machine)
  - State machine patterns from LangGraph/CrewAI

  **Acceptance Criteria**:
  - [ ] Start() validates dependencies before transition
  - [ ] Complete() requires result string
  - [ ] Fail() increments retry_count
  - [ ] Block() and Unblock() work correctly
  - [ ] Circular dependency detection prevents deadlocks
  - [ ] Terminal states (completed) reject all transitions

  **QA Scenarios**:
  ```
  Scenario: Cannot start todo with unmet dependencies
    Tool: Bash (go test)
    Steps:
      1. Create todo A with status "pending"
      2. Create todo B with depends_on = [A.id]
      3. Attempt Start(B.id)
    Expected Result: Error returned "dependency not satisfied"
    Evidence: .sisyphus/evidence/task-05-dep-check.txt

  Scenario: Circular dependency detection
    Tool: Bash (go test)
    Steps:
      1. Create todo A with depends_on = [B.id]
      2. Create todo B with depends_on = [A.id]
      3. Attempt Start(A.id) or Start(B.id)
    Expected Result: Error returned "circular dependency detected"
    Evidence: .sisyphus/evidence/task-05-circular-dep.txt

  Scenario: Complete transition works with result
    Tool: Bash (go test)
    Steps:
      1. Create todo, Start it, then Complete with result
      2. Verify status is "completed" and result is stored
    Expected Result: Todo status = completed, result field populated
    Evidence: .sisyphus/evidence/task-05-complete.txt
  ```

  **Commit**: YES
  - Message: `feat(todo): add state machine validation`
  - Files: `server/internal/todo/manager.go`
  - Pre-commit: `go test ./internal/todo/... -v`

- [ ] 6. MCP Tool Implementation

  **What to do**:
  - Create `manage_todo` tool in `server/internal/tools/todo_tool.go`
  - Follow existing Tool interface pattern exactly
  - Define tool with actions: create, start, complete, fail, list, replan
  - Implement JSON Schema for parameters

  **Tool Definition**:
  ```go
  type TodoTool struct {
      todoMgr todo.TodoManager
  }

  func NewTodoTool(todoMgr todo.TodoManager) *TodoTool {
      return &TodoTool{todoMgr: todoMgr}
  }

  func (t *TodoTool) GetDefinition() openai.Tool {
      return openai.Tool{
          Type: "function",
          Function: &openai.FunctionDefinition{
              Name:        "manage_todo",
              Description: "Manage todo list for task planning and execution tracking",
              Parameters: map[string]any{
                  "type": "object",
                  "properties": map[string]any{
                      "action": map[string]any{
                          "type": "string",
                          "enum": []string{"create", "start", "complete", "fail", "list", "replan"},
                          "description": "Action to perform",
                      },
                      "todos": map[string]any{
                          "type": "array",
                          "description": "Todo items (for create/replan)",
                          "items": map[string]any{
                              "type": "object",
                              "properties": map[string]any{
                                  "content":    map[string]any{"type": "string"},
                                  "validation": map[string]any{"type": "string"},
                                  "depends_on": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                              },
                              "required": []string{"content"},
                          },
                      },
                      "todo_id": map[string]any{
                          "type": "string",
                          "description": "Todo ID (for start/complete/fail)",
                      },
                      "result": map[string]any{
                          "type": "string",
                          "description": "Completion evidence (for complete)",
                      },
                      "reason": map[string]any{
                          "type": "string",
                          "description": "Failure/block reason",
                      },
                  },
                  "required": []string{"action"},
              },
          },
      }
  }

  func (t *TodoTool) Execute(ctx context.Context, args map[string]any) (string, error) {
      action := args["action"].(string)
      switch action {
      case "create":
          return t.handleCreate(ctx, args)
      case "start":
          return t.handleStart(ctx, args)
      case "complete":
          return t.handleComplete(ctx, args)
      case "fail":
          return t.handleFail(ctx, args)
      case "list":
          return t.handleList(ctx, args)
      case "replan":
          return t.handleReplan(ctx, args)
      default:
          return "", fmt.Errorf("invalid action: %s", action)
      }
  }
  ```

  **Must NOT do**:
  - Do NOT allow actions beyond defined set
  - Do NOT skip parameter validation
  - Do NOT return unstructured errors (JSON error response)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Single tool implementation following existing patterns
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7, 8, 9, 10)
  - **Blocks**: Task 9 (registration)
  - **Blocked By**: Tasks 1, 4 (needs TodoManager and schema)

  **References**:
  - `server/internal/tools/code_tool.go` - Existing tool implementation pattern
  - `server/internal/tools/shell_tool.go` - Tool Execute pattern
  - `server/internal/tool/manager.go:1-15` - Tool interface

  **Acceptance Criteria**:
  - [ ] Tool registered with name "manage_todo"
  - [ ] All 6 actions implemented (create, start, complete, fail, list, replan)
  - [ ] JSON Schema validates input parameters
  - [ ] Execute returns JSON string response

  **QA Scenarios**:
  ```
  Scenario: Create action creates todo
    Tool: Bash (go test)
    Steps:
      1. Create session
      2. Call tool.Execute with action="create", todos=[{content: "Test task"}]
      3. Verify todo exists in database
    Expected Result: JSON response with created todo ID and status "pending"
    Evidence: .sisyphus/evidence/task-06-tool-create.txt

  Scenario: Complete action requires result
    Tool: Bash (go test)
    Steps:
      1. Create todo, Start it
      2. Call tool.Execute with action="complete", todo_id=..., result=""
    Expected Result: Error "result is required for complete action"
    Evidence: .sisyphus/evidence/task-06-tool-complete-validation.txt

  Scenario: List action returns todos
    Tool: Bash (go test)
    Steps:
      1. Create session with 3 todos
      2. Call tool.Execute with action="list", session_id=...
    Expected Result: JSON array with 3 todos
    Evidence: .sisyphus/evidence/task-06-tool-list.txt
  ```

  **Commit**: YES
  - Message: `feat(tool): add manage_todo MCP tool`
  - Files: `server/internal/tools/todo_tool.go`

- [ ] 7. REST API Endpoints

  **What to do**:
  - Add todo endpoints in `server/internal/api/handlers.go`
  - Register routes in `server/internal/api/routes.go`
  - Follow existing endpoint patterns (error handling, JSON response)

  **Endpoints**:
  ```go
  // GET /api/sessions/:id/todos - List todos for session
  func (h *Handlers) GetSessionTodos(c *gin.Context) {
      sessionID := c.Param("id")
      todos, err := h.todoMgr.List(c.Request.Context(), sessionID)
      if err != nil {
          c.JSON(500, gin.H{"error": err.Error()})
          return
      }
      c.JSON(200, todos)
  }

  // POST /api/sessions/:id/todos - Create todo
  func (h *Handlers) CreateTodo(c *gin.Context) {
      sessionID := c.Param("id")
      var req struct {
          Content    string   `json:"content" binding:"required"`
          ActiveForm string   `json:"active_form"`
          DependsOn  []string `json:"depends_on"`
          Validation string   `json:"validation"`
      }
      if err := c.ShouldBindJSON(&req); err != nil {
          c.JSON(400, gin.H{"error": err.Error()})
          return
      }
      // ... create todo
  }

  // PATCH /api/sessions/:id/todos/:todo_id - Update todo status
  // DELETE /api/sessions/:id/todos/:todo_id - Delete todo
  ```

  **Routes**:
  ```go
  sessions := api.Group("/sessions")
  sessions.GET("/:id/todos", h.GetSessionTodos)
  sessions.POST("/:id/todos", h.CreateTodo)
  sessions.PATCH("/:id/todos/:todo_id", h.UpdateTodo)
  sessions.DELETE("/:id/todos/:todo_id", h.DeleteTodo)
  ```

  **Must NOT do**:
  - Do NOT bypass TodoManager (must use h.todoMgr)
  - Do NOT allow updates to completed todos
  - Do NOT return internal errors to client (wrap appropriately)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Standard REST endpoints following existing handlers
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 6, 8, 9, 10)
  - **Blocks**: Task 8 (SSE events need endpoints)
  - **Blocked By**: Tasks 3, 4 (needs TodoManager interface)

  **References**:
  - `server/internal/api/handlers.go:32-74` - CreateSession handler pattern
  - `server/internal/api/handlers.go:76-106` - GetSession handler pattern
  - `server/internal/api/routes.go` - Route registration pattern

  **Acceptance Criteria**:
  - [ ] GET /api/sessions/:id/todos returns todo list
  - [ ] POST /api/sessions/:id/todos creates todo
  - [ ] PATCH /api/sessions/:id/todos/:todo_id updates status
  - [ ] DELETE /api/sessions/:id/todos/:todo_id removes todo
  - [ ] 404 returned for invalid session/todo IDs

  **QA Scenarios**:
  ```
  Scenario: GET todos returns correct list
    Tool: Bash (curl)
    Steps:
      1. Create session via POST /api/sessions
      2. Create 2 todos via POST
      3. GET /api/sessions/{session-id}/todos
    Expected Result: JSON array with 2 todos, status 200
    Evidence: .sisyphus/evidence/task-07-api-get.txt

  Scenario: POST creates todo with required fields
    Tool: Bash (curl)
    Steps:
      1. POST /api/sessions/{session-id}/todos with {"content":"Test task"}
    Expected Result: JSON with created todo, status 201, id field present
    Evidence: .sisyphus/evidence/task-07-api-create.txt

  Scenario: Invalid session returns 404
    Tool: Bash (curl)
    Steps:
      1. GET /api/sessions/invalid-uuid/todos
    Expected Result: status 404 with error message
    Evidence: .sisyphus/evidence/task-07-api-404.txt
  ```

  **Commit**: YES
  - Message: `feat(api): add todo REST endpoints`
  - Files: `server/internal/api/handlers.go`, `server/internal/api/routes.go`

- [ ] 8. SSE Event Integration

  **What to do**:
  - Add todo-related SSE events to Agent Engine
  - Broadcast events when todo state changes
  - Follow existing SSE event pattern (message, tool_call, done)

  **Event Types**:
  ```go
  // Add to SSE event stream
  const (
      EventTypeTodoCreated       = "todo_created"
      EventTypeTodoStatusChanged = "todo_status_changed"
      EventTypeTodoCompleted     = "todo_completed"
      EventTypeTodoDeleted       = "todo_deleted"
      EventTypeTodoReplan        = "todo_replan"
  )

  // Event payload
  type TodoEvent struct {
      ID      string     `json:"id"`
      Content string     `json:"content"`
      Status  string     `json:"status"`
      From    string     `json:"from,omitempty"` // For status_changed
      To      string     `json:"to,omitempty"`
      Result  string     `json:"result,omitempty"`
  }
  ```

  **Integration in TodoManager**:
  ```go
  func (m *todoManager) Complete(ctx context.Context, id string, result string) (*session.Todo, error) {
      todo, err := m.Update(ctx, id, map[string]any{
          "status":     session.TodoStatusCompleted,
          "result":     result,
          "completed_at": time.Now(),
      })
      if err != nil {
          return nil, err
      }
      
      // Broadcast SSE event
      m.eventBus.Publish(EventTypeTodoCompleted, TodoEvent{
          ID:     todo.ID.String(),
          Status: "completed",
          Result: result,
      })
      
      return todo, nil
  }
  ```

  **Must NOT do**:
  - Do NOT add WebSocket (use existing SSE pattern)
  - Do NOT broadcast events for every update (only state changes)
  - Do NOT include sensitive data in events

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Follows existing SSE pattern in Agent Engine
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 6, 7, 9, 10)
  - **Blocks**: Task 14 (frontend SSE subscription)
  - **Blocked By**: Task 7 (needs endpoints)

  **References**:
  - `server/internal/agent/engine.go:23-66` - SSE event streaming pattern
  - `server/internal/agent/engine.go:116-321` - Event broadcasting in agent loop

  **Acceptance Criteria**:
  - [ ] todo_created event fired when todo created
  - [ ] todo_status_changed event fired on state transitions
  - [ ] todo_completed event includes result field
  - [ ] Events broadcast via existing SSE mechanism

  **QA Scenarios**:
  ```
  Scenario: SSE event on todo creation
    Tool: Bash (curl with SSE)
    Steps:
      1. curl -N http://localhost:8080/api/sessions/{id}/chat
      2. Agent creates todo via tool
      3. Verify SSE event received: event: todo_created
    Expected Result: Event payload contains todo id, content, status="pending"
    Evidence: .sisyphus/evidence/task-08-sse-created.txt

  Scenario: SSE event on status change
    Tool: Bash (curl with SSE)
    Steps:
      1. Subscribe to SSE stream
      2. Agent calls Start on todo
      3. Verify event: todo_status_changed with from="pending" to="in_progress"
    Expected Result: Event contains both from and to status fields
    Evidence: .sisyphus/evidence/task-08-sse-status.txt
  ```

  **Commit**: YES
  - Message: `feat(sse): add todo event streaming`
  - Files: `server/internal/todo/manager.go`, `server/internal/agent/engine.go`

- [ ] 9. Tool Registration in Agent Engine

  **What to do**:
  - Register TodoTool in Tool Manager
  - Add to OpenAI tool definitions returned to LLM
  - Wire TodoManager in main.go dependency injection

  **Registration**:
  ```go
  // In cmd/server/main.go
  todoMgr := todo.NewTodoManager(db)
  todoTool := tools.NewTodoTool(todoMgr)
  toolMgr.Register(todoTool)

  // In tool manager GetOpenAITools()
  func (m *Manager) GetOpenAITools() []openai.Tool {
      tools := []openai.Tool{}
      for _, tool := range m.tools {
          tools = append(tools, tool.GetDefinition())
      }
      return tools
  }
  ```

  **Must NOT do**:
  - Do NOT register tool twice
  - Do NOT skip dependency injection
  - Do NOT hardcode session ID in tool (must be passed from agent context)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple registration following existing pattern
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 6, 7, 8, 10)
  - **Blocks**: F3 (integration test)
  - **Blocked By**: Task 6 (needs tool implementation)

  **References**:
  - `server/cmd/server/main.go:41-51` - Existing tool registration pattern
  - `server/internal/tool/manager.go` - Tool registration interface

  **Acceptance Criteria**:
  - [ ] TodoTool registered in ToolManager
  - [ ] "manage_todo" appears in OpenAI tool definitions
  - [ ] TodoManager injected via dependency injection
  - [ ] Tool receives session context from Agent Engine

  **QA Scenarios**:
  ```
  Scenario: Tool appears in LLM tool list
    Tool: Bash (curl + grep)
    Steps:
      1. Send chat request to session
      2. Inspect tool definitions in response (or debug log)
    Expected Result: "manage_todo" function definition present
    Evidence: .sisyphus/evidence/task-09-tool-list.txt

  Scenario: Tool execution works end-to-end
    Tool: Bash (curl)
    Steps:
      1. POST /api/sessions/{id}/chat with "帮我规划一个任务"
      2. Verify agent creates todos via tool
    Expected Result: Todos created, SSE events broadcast
    Evidence: .sisyphus/evidence/task-09-integration.txt
  ```

  **Commit**: YES
  - Message: `feat(tool): register manage_todo tool in agent engine`
  - Files: `server/cmd/server/main.go`, `server/internal/tool/manager.go`

- [ ] 10. Backend Tests

  **What to do**:
  - Write comprehensive tests for TodoManager
  - Write tests for state transitions
  - Write tests for TodoTool
  - Use testify/assert and testify/require
  - Use setupTestDB pattern with t.Cleanup()

  **Test Files**:
  - `server/internal/todo/manager_test.go` - TodoManager tests
  - `server/internal/tools/todo_tool_test.go` - Tool tests
  - `server/internal/api/todo_handlers_test.go` - API tests

  **Test Coverage Target**:
  - TodoManager: 80%+ coverage
  - State transitions: 100% coverage (all transitions tested)
  - Tool: 80%+ coverage

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Comprehensive test suite, multiple files
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 6, 7, 8, 9)
  - **Blocks**: None (tests don't block other tasks)
  - **Blocked By**: Tasks 4, 6, 7 (needs implementation)

  **References**:
  - `server/internal/session/manager_test.go` - Test pattern with setupTestDB
  - `server/internal/tool/manager_test.go` - Tool test patterns

  **Acceptance Criteria**:
  - [ ] `go test ./internal/todo/... -v` passes with 0 failures
  - [ ] `go test ./internal/tools/todo_tool_test.go -v` passes
  - [ ] Coverage >= 80% for TodoManager

  **QA Scenarios**:
  ```
  Scenario: All tests pass
    Tool: Bash (go test)
    Steps:
      1. cd server && go test ./internal/todo/... ./internal/tools/... -v -cover
    Expected Result: PASS with coverage report >= 80%
    Evidence: .sisyphus/evidence/task-10-tests.txt
  ```

  **Commit**: YES
  - Message: `test(todo): add comprehensive test coverage`
  - Files: `server/internal/todo/manager_test.go`, `server/internal/tools/todo_tool_test.go`
  - Pre-commit: `go test ./... -v`

- [ ] 11. TodoItem Component

  **What to do**:
  - Create `TodoItem` component in `packages/ui/src/components/TodoItem/`
  - Display todo with status icon, content, and actions
  - Export from `packages/ui/src/index.ts`

  **Component**:
  ```tsx
  // packages/ui/src/components/TodoItem/index.tsx
  import React from 'react';
  import { CheckCircleOutlined, LoadingOutlined, CircleOutlined, LockOutlined, CloseCircleOutlined } from '@ant-design/icons';

  export interface TodoItemProps {
    id: string;
    content: string;
    status: 'pending' | 'in_progress' | 'completed' | 'blocked' | 'failed';
    activeForm?: string;
    result?: string;
    onStatusChange?: (id: string, status: string) => void;
    readonly?: boolean;
  }

  const statusIcons = {
    pending: <CircleOutlined />,
    in_progress: <LoadingOutlined spin />,
    completed: <CheckCircleOutlined style={{ color: '#52c41a' }} />,
    blocked: <LockOutlined />,
    failed: <CloseCircleOutlined style={{ color: '#ff4d4f' }} />,
  };

  export const TodoItem: React.FC<TodoItemProps> = ({
    id,
    content,
    status,
    activeForm,
    result,
    onStatusChange,
    readonly = false,
  }) => {
    const displayText = status === 'in_progress' && activeForm ? activeForm : content;

    return (
      <div className="todo-item" data-status={status}>
        <div className="todo-status-icon">
          {statusIcons[status]}
        </div>
        <div className="todo-content">
          <span>{displayText}</span>
          {result && <div className="todo-result">{result}</div>}
        </div>
        {!readonly && onStatusChange && (
          <div className="todo-actions">
            {/* Action buttons based on status */}
          </div>
        )}
      </div>
    );
  };
  ```

  **Must NOT do**:
  - Do NOT use `any` type (strict TypeScript)
  - Do NOT inline styles (use CSS classes or styled-components)
  - Do NOT add features beyond spec (no priority, no tags)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: UI component with styling and interaction
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 12, 13, 14, 15)
  - **Blocks**: Task 12 (TodoList uses TodoItem)
  - **Blocked By**: Task 8 (needs SSE events defined)

  **References**:
  - `packages/ui/src/components/Bubble/index.tsx` - Component structure pattern
  - `packages/demo/src/App.tsx` - @ant-design/x usage patterns

  **Acceptance Criteria**:
  - [ ] TodoItem renders with correct status icon
  - [ ] Displays activeForm when status is in_progress
  - [ ] Shows result when status is completed
  - [ ] Exported from packages/ui/src/index.ts

  **QA Scenarios**:
  ```
  Scenario: Renders with correct status icons
    Tool: Playwright (playwright skill)
    Steps:
      1. Render TodoItem with status="pending"
      2. Verify CircleOutlined icon present
      3. Render with status="completed"
      4. Verify CheckCircleOutlined with green color
    Expected Result: Correct icon for each status
    Evidence: .sisyphus/evidence/task-11-status-icons.png

  Scenario: Displays active form when in progress
    Tool: Playwright (playwright skill)
    Steps:
      1. Render TodoItem with status="in_progress", activeForm="Implementing..."
      2. Verify text is "Implementing..." not content
    Expected Result: Active form displayed instead of content
    Evidence: .sisyphus/evidence/task-11-active-form.png
  ```

  **Commit**: YES
  - Message: `feat(ui): add TodoItem component`
  - Files: `packages/ui/src/components/TodoItem/index.tsx`, `packages/ui/src/index.ts`

- [ ] 12. TodoList Component

  **What to do**:
  - Create `TodoList` component in `packages/ui/src/components/TodoList/`
  - Display list of TodoItems with sorting
  - Handle empty state
  - Export from `packages/ui/src/index.ts`

  **Component**:
  ```tsx
  // packages/ui/src/components/TodoList/index.tsx
  import React from 'react';
  import { TodoItem, TodoItemProps } from '../TodoItem';

  export interface TodoListProps {
    todos: TodoItemProps[];
    onStatusChange?: (id: string, status: string) => void;
    readonly?: boolean;
    emptyText?: string;
  }

  export const TodoList: React.FC<TodoListProps> = ({
    todos,
    onStatusChange,
    readonly = false,
    emptyText = 'No tasks yet',
  }) => {
    if (todos.length === 0) {
      return <div className="todo-empty">{emptyText}</div>;
    }

    // Sort: in_progress first, then pending, then completed/blocked/failed
    const sortedTodos = [...todos].sort((a, b) => {
      const order = { in_progress: 0, pending: 1, blocked: 2, failed: 3, completed: 4 };
      return order[a.status] - order[b.status];
    });

    return (
      <div className="todo-list" data-testid="todo-list">
        {sortedTodos.map((todo) => (
          <TodoItem
            key={todo.id}
            {...todo}
            onStatusChange={onStatusChange}
            readonly={readonly}
          />
        ))}
      </div>
    );
  };
  ```

  **Must NOT do**:
  - Do NOT use `any` type
  - Do NOT add virtualization (MVP, <100 items expected)
  - Do NOT add filtering (MVP)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: UI list component with sorting logic
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 11, 13, 14, 15)
  - **Blocks**: Task 13 (sidebar uses TodoList)
  - **Blocked By**: Task 11 (needs TodoItem)

  **References**:
  - `packages/ui/src/components/BubbleList/index.tsx` - List component pattern

  **Acceptance Criteria**:
  - [ ] TodoList renders all todos
  - [ ] Todos sorted by status (in_progress first)
  - [ ] Empty state displays when no todos
  - [ ] Exported from packages/ui/src/index.ts

  **QA Scenarios**:
  ```
  Scenario: Renders sorted todo list
    Tool: Playwright (playwright skill)
    Steps:
      1. Render TodoList with 3 todos: pending, in_progress, completed
      2. Verify order: in_progress first, then pending, then completed
    Expected Result: Correct sort order
    Evidence: .sisyphus/evidence/task-12-sorted-list.png

  Scenario: Empty state when no todos
    Tool: Playwright (playwright skill)
    Steps:
      1. Render TodoList with empty array
      2. Verify "No tasks yet" text displayed
    Expected Result: Empty state message shown
    Evidence: .sisyphus/evidence/task-12-empty-state.png
  ```

  **Commit**: YES
  - Message: `feat(ui): add TodoList component`
  - Files: `packages/ui/src/components/TodoList/index.tsx`, `packages/ui/src/index.ts`

- [ ] 13. Sidebar Layout Integration

  **What to do**:
  - Add todo sidebar to demo app layout
  - Position sidebar on right side
  - Make sidebar resizable (optional, nice-to-have)
  - Integrate with session context

  **Layout**:
  ```tsx
  // packages/demo/src/App.tsx (modifications)
  import { TodoList } from '@copcon/ui';

  function App() {
    const [todos, setTodos] = useState<Todo[]>([]);
    const [sessionId, setSessionId] = useState<string>();

    return (
      <div className="app-layout">
        <main className="chat-area">
          {/* Existing chat UI */}
        </main>
        <aside className="todo-sidebar">
          <h3>Tasks</h3>
          <TodoList
            todos={todos}
            onStatusChange={handleStatusChange}
          />
        </aside>
      </div>
    );
  }
  ```

  **Must NOT do**:
  - Do NOT make sidebar required (should be toggleable)
  - Do NOT hardcode session ID (use state)
  - Do NOT add complex layout logic (keep simple)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: Layout integration with CSS
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 11, 12, 14, 15)
  - **Blocks**: None
  - **Blocked By**: Task 12 (needs TodoList)

  **References**:
  - `packages/demo/src/App.tsx` - Current layout structure

  **Acceptance Criteria**:
  - [ ] Sidebar renders on right side
  - [ ] TodoList integrated in sidebar
  - [ ] Layout responsive (sidebar collapsible on mobile)

  **QA Scenarios**:
  ```
  Scenario: Sidebar renders with todo list
    Tool: Playwright (playwright skill)
    Steps:
      1. Navigate to demo app
      2. Verify sidebar present on right side
      3. Verify TodoList inside sidebar
    Expected Result: Sidebar with TodoList visible
    Evidence: .sisyphus/evidence/task-13-sidebar.png

  Scenario: Sidebar toggle works
    Tool: Playwright (playwright skill)
    Steps:
      1. Click sidebar toggle button
      2. Verify sidebar hidden
      3. Click again, verify sidebar shown
    Expected Result: Sidebar toggles visibility
    Evidence: .sisyphus/evidence/task-13-toggle.png
  ```

  **Commit**: YES
  - Message: `feat(ui): integrate todo sidebar in demo`
  - Files: `packages/demo/src/App.tsx`, `packages/demo/src/App.css`

- [ ] 14. SSE Event Subscription

  **What to do**:
  - Subscribe to todo SSE events in frontend
  - Update todo list state on events
  - Handle connection errors gracefully

  **Event Subscription**:
  ```tsx
  // packages/demo/src/hooks/useTodoEvents.ts
  import { useEffect } from 'react';

  export function useTodoEvents(sessionId: string, onEvent: (event: TodoEvent) => void) {
    useEffect(() => {
      const eventSource = new EventSource(`/api/sessions/${sessionId}/chat`);

      eventSource.addEventListener('todo_created', (e) => {
        const data = JSON.parse(e.data);
        onEvent({ type: 'created', ...data });
      });

      eventSource.addEventListener('todo_status_changed', (e) => {
        const data = JSON.parse(e.data);
        onEvent({ type: 'status_changed', ...data });
      });

      eventSource.addEventListener('todo_completed', (e) => {
        const data = JSON.parse(e.data);
        onEvent({ type: 'completed', ...data });
      });

      return () => eventSource.close();
    }, [sessionId, onEvent]);
  }
  ```

  **Must NOT do**:
  - Do NOT poll (use SSE)
  - Do NOT reconnect infinitely (max retries)
  - Do NOT block UI on event processing

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: Real-time event handling
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 11, 12, 13, 15)
  - **Blocks**: None
  - **Blocked By**: Task 8 (needs SSE events defined)

  **References**:
  - `packages/ui/src/api/agentClient.ts` - Existing API client pattern

  **Acceptance Criteria**:
  - [ ] SSE connection established on session load
  - [ ] todo_created event adds todo to list
  - [ ] todo_status_changed event updates todo status
  - [ ] todo_completed event updates todo with result
  - [ ] Connection errors handled gracefully

  **QA Scenarios**:
  ```
  Scenario: Real-time todo creation
    Tool: Playwright (playwright skill)
    Steps:
      1. Load demo app with session
      2. Send message that creates todo via agent
      3. Verify todo appears in sidebar without refresh
    Expected Result: Todo appears within 2 seconds
    Evidence: .sisyphus/evidence/task-14-realtime-create.png

  Scenario: Status change updates UI
    Tool: Playwright (playwright skill)
    Steps:
      1. Have todo in pending state
      2. Agent starts todo (status → in_progress)
      3. Verify icon changes to loading spinner
    Expected Result: Status icon updates immediately
    Evidence: .sisyphus/evidence/task-14-status-update.png
  ```

  **Commit**: YES
  - Message: `feat(ui): add SSE event subscription for todos`
  - Files: `packages/demo/src/hooks/useTodoEvents.ts`, `packages/demo/src/App.tsx`

- [ ] 15. Frontend Tests

  **What to do**:
  - Write tests for TodoItem component
  - Write tests for TodoList component
  - Write tests for useTodoEvents hook
  - Use React Testing Library

  **Test Files**:
  - `packages/ui/src/components/TodoItem/index.test.tsx`
  - `packages/ui/src/components/TodoList/index.test.tsx`
  - `packages/demo/src/hooks/useTodoEvents.test.ts`

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: React component tests
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 11, 12, 13, 14)
  - **Blocks**: None
  - **Blocked By**: Tasks 11, 12, 14 (needs components and hook)

  **References**:
  - `packages/ui/src/components/Bubble/index.test.tsx` - Existing test patterns

  **Acceptance Criteria**:
  - [ ] `pnpm test` passes for all todo-related tests
  - [ ] TodoItem tests cover all status states
  - [ ] TodoList tests cover sorting and empty state
  - [ ] useTodoEvents tests cover event handling

  **QA Scenarios**:
  ```
  Scenario: All frontend tests pass
    Tool: Bash (pnpm test)
    Steps:
      1. cd packages/ui && pnpm test
      2. cd packages/demo && pnpm test
    Expected Result: All tests pass
    Evidence: .sisyphus/evidence/task-15-frontend-tests.txt
  ```

  **Commit**: YES
  - Message: `test(ui): add tests for todo components`
  - Files: `packages/ui/src/components/TodoItem/index.test.tsx`, `packages/ui/src/components/TodoList/index.test.tsx`
  - Pre-commit: `pnpm test`

---

## Final Verification Wave

4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists. For each "Must NOT Have": search codebase for forbidden patterns. Check evidence files exist in .sisyphus/evidence/.

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go test ./... -v` + frontend tests. Review all changed files for: `as any`, empty catches, unused imports. Check AI slop: excessive comments, over-abstraction.

- [ ] F3. **Real Manual QA** — `unspecified-high` (+ `playwright` skill)
  Start from clean state. Execute EVERY QA scenario from EVERY task. Test edge cases: empty list, failed todos, circular dependencies. Save evidence to .sisyphus/evidence/final-qa/.

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1 — everything in spec was built, nothing beyond spec. Check "Must NOT do" compliance.

---

## Commit Strategy

- **1**: `feat(todo): add database schema and Todo model`
- **2**: `feat(todo): implement TodoManager with CRUD operations`
- **3**: `feat(todo): add state machine validation`
- **4**: `feat(tool): add manage_todo MCP tool`
- **5**: `feat(api): add todo REST endpoints`
- **6**: `feat(sse): add todo event streaming`
- **7**: `feat(ui): add TodoItem and TodoList components`
- **8**: `feat(ui): integrate todo sidebar in demo`
- **9**: `test(todo): add comprehensive test coverage`

---

## Success Criteria

### Verification Commands

```bash
# Backend tests
cd server && go test ./internal/todo/... -v
cd server && go test ./internal/tools/todo_tool_test.go -v

# Frontend tests
cd packages/ui && pnpm test

# Integration
curl -X POST http://localhost:8080/api/sessions
curl -X POST http://localhost:8080/api/sessions/{session-id}/todos -d '{"content":"Test task"}'
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass (backend + frontend)
- [ ] Todo sidebar renders and updates in real-time
- [ ] LLM can create/complete todos via tool
