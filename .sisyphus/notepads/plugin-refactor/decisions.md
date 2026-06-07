# Plugin Refactor - Decisions

## Architecture Decisions
- Strangler fig pattern: new Plugin system alongside old capabilities, delete old at end (Task 10)
- Two-phase init: Register() stores plugin ref, Build() calls Init(deps)
- Plugin naming: `{namespace}.{type}.{name}` e.g. `builtin.tool.code_executor`
- ToolPool supports namespace wildcards: `namespace.*` matches all tools with that prefix
