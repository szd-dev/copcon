# Agent Registry Implementation

## Task 4: AgentRegistry Interface Definition

### Files Created

1. **server/internal/agent/registry.go**
   - Defines `AgentRegistry` interface with methods: `Get(id string)`, `List() []AgentInfo`, `Default()`
   - Defines `AgentDefinition` struct with fields: ID, Name, Model, SystemPrompt, ToolManager, OpenAIClient
   - Defines `AgentInfo` struct with fields: ID, Name
   - Defines sentinel errors: `ErrAgentNotFound`, `ErrNoDefaultAgent`

2. **server/internal/agent/registry_test.go**
   - TDD approach: Tests written first, then implementation
   - Tests cover: AgentDefinition struct creation, AgentRegistry interface methods, AgentInfo equality
   - Includes mock ToolManager implementation for testing
   - Includes in-memory test implementation of AgentRegistry

### Test Results

```
=== RUN   TestAgentRegistry
--- PASS: TestAgentRegistry (0.00s)
=== RUN   TestAgentInfoEquality
--- PASS: TestAgentInfoEquality (0.00s)
PASS
ok      github.com/copcon/server/internal/agent 0.007s
```

### Key Design Decisions

- `AgentDefinition` holds all configuration needed for an agent instance
- `AgentInfo` is a lightweight struct for listing agents (without full config)
- `AgentRegistry` interface allows for different implementations (in-memory, database-backed, etc.)
- Error handling follows Go conventions with sentinel errors
- OpenAI client type uses `openai.Client` from go-openai library

### Dependencies

- `github.com/openai/openai-go/v3` - for OpenAI client type
- `github.com/copcon/server/internal/tool` - for ToolManager interface

### Acceptance Criteria

- [x] AgentRegistry interface defined
- [x] AgentDefinition structure defined
- [x] Unit tests pass

---

## Task 9: AgentRegistry Implementation

### Implementation Summary

Implemented `agentRegistry` struct that satisfies the `AgentRegistry` interface defined in Task 4.

### Files Modified

- `server/internal/agent/registry.go` - Added full implementation
- `server/internal/agent/registry_test.go` - Updated with comprehensive tests

### Key Features

1. **Config Loading**: Loads agent definitions from `config.Config.Agents`
2. **Tool Validation**: Validates that all tool names referenced by agents exist in the ToolRegistry
3. **Per-Agent ToolManager**: Creates a separate ToolManager for each agent containing only the tools specified in its config
4. **OpenAI Client Creation**: Creates an OpenAI client for each agent with proper base URL handling (agent-specific overrides global)
5. **Thread-Safe**: Uses `sync.RWMutex` for concurrent access safety

### Implementation Details

```go
type agentRegistry struct {
    mu           sync.RWMutex
    agents       map[string]AgentDefinition
    defaultAgent string
}
```

### Methods Implemented

- `NewAgentRegistry(cfg *config.Config, toolRegistry tool.ToolRegistry) (AgentRegistry, error)` - Constructor
- `Get(id string) (AgentDefinition, error)` - Returns agent by ID or ErrAgentNotFound
- `List() []AgentInfo` - Returns list of all agent info (ID + Name)
- `Default() (AgentDefinition, error)` - Returns default agent or ErrNoDefaultAgent

### Error Handling

- `ErrAgentNotFound` - Returned when agent ID doesn't exist
- `ErrNoDefaultAgent` - Returned when no default agent is configured
- Tool validation errors include agent ID and tool name for debugging

### Test Coverage

All tests pass (7/7):
- TestAgentRegistryLoad - Validates loading agents from config
- TestAgentRegistryGet - Tests Get() method including non-existent agents
- TestAgentRegistryDefault - Tests Default() with and without default configured
- TestAgentRegistryValidateTools - Tests tool name validation (valid, invalid, mixed)
- TestAgentRegistryEmpty - Tests behavior with empty agent list
- TestAgentDefinitionStruct - Validates AgentDefinition struct
- TestAgentInfoEquality - Validates AgentInfo comparison

### QA Scenario Evidence

Tool validation test saved to: `.sisyphus/evidence/task-09-invalid-tool.txt`

The test confirms that attempting to create an AgentRegistry with an agent referencing a non-existent tool returns an error with message containing "tool not found".

### Dependencies

- Task 1 (Config) - Uses `config.Config` and `config.AgentConfig`
- Task 4 (Registry Interface) - Implements `AgentRegistry` interface
- Task 7 (ToolRegistry) - Uses `tool.ToolRegistry` for tool validation and `tool.NewToolManager()`

### Blocking

This task blocks:
- Task 10 (AgentEngine refactor to use AgentRegistry)
- Task 11 (AgentEngine integration)
- Task 16 (Multi-agent support)
