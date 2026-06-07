# Learnings: skill-capability

## 2026-06-06T06:14 Session Start

### Project Conventions
- plugins go.mod already has `gopkg.in/yaml.v3` (direct)
- core go.mod has `gopkg.in/yaml.v3` (indirect, via openai-go)
- plugins module: `github.com/copcon/plugins`, core module: `github.com/copcon/core`
- Test framework: `github.com/stretchr/testify v1.11.1`
- Test patterns: testify/assert+require, hand-written mocks, 1-func-1-scenario, t.TempDir()
- Hook naming: `snake_case` (e.g. `skill_info`, `todo_injection`, `kb_info`)
- Module naming: `modules.xxx` (e.g. `modules.memory_file`, `modules.kb`)
- Tool naming: `snake_case` (e.g. `code_executor`, `shell_executor`, `kb_search`)

### Architecture Constraints
- CapabilityTypeSkill is a DEAD END — harness doesn't handle it
- SkillModule.Type() MUST return CapabilityTypeModule
- ModuleCapability is checked BEFORE type switch in harness (engine.go:197-222)
- OnSystemPrompt hooks: todo_injection(50) → kb_info(55) → skill_info(60) → memory(80)
