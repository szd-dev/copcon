
## Task 8: Replace GetOpenAITools() → GetToolDefs()

### What was done
- Replaced `GetOpenAITools() []openai.ChatCompletionToolUnionParam` with `GetToolDefs() []llm.ToolDef` in ToolManager interface
- Implementation in toolManager now creates `llm.ToolDef` structs directly (json.Marshal the schema map into `Parameters json.RawMessage`)
- Removed `openai` import from `tool/manager.go`, added `llm` import
- Removed `convertToLLMTools()` function from `agent/engine.go` — no longer needed since tools are already `[]llm.ToolDef`
- Removed `openai` and `encoding/json` imports from `agent/engine.go`
- Updated 3 test mocks: `mockToolManagerForEngine`, `mockToolManagerWithTools`, `registryMockToolManager`
- Removed `openai` imports from 3 test files, added `llm` import where needed
- Renamed test `TestToolManager_GetOpenAITools` → `TestToolManager_GetToolDefs`

### Key insight
The existing `convertTools()` in `llm/openai_adapter.go` already handles `[]llm.ToolDef → []openai.ChatCompletionToolUnionParam` conversion. No new adapter function was needed — the flow just reversed: instead of ToolManager→OpenAI→ToolDef, it's now ToolManager→ToolDef→(adapter converts to OpenAI at call time).

### Files changed
- `server/internal/tool/manager.go` — interface + implementation
- `server/internal/agent/engine.go` — caller + removed convertToLLMTools
- `server/internal/agent/engine_test.go` — mock update
- `server/internal/agent/engine_execution_test.go` — mock update
- `server/internal/agent/registry_test.go` — mock update
- `server/internal/tool/manager_test.go` — test rename + method update
