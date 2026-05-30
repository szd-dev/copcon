package capabilities

// ---- Capability Names: Hooks ----

const (
	HookTodoInjection = "hooks.todo_injection"
	HookMemory        = "hooks.memory"
	HookLogging       = "hooks.logging"
	HookTracing       = "hooks.tracing"
	HookFileMemory    = "hooks.file_memory"
	HookKBRecall      = "hooks.kb_recall"
	HookMemoryPersist = "hooks.memory_persist"
)

// ---- Capability Names: Modules ----

const (
	CapMemoryFile = "modules.memory_file"
)

// ---- Capability Names: Tools ----

const (
	ToolConfirmAction  = "tools.confirm_action"
	ToolAskUser        = "tools.ask_user"
	ToolTodo           = "tools.todo"
	ToolAsync          = "tools.async"
	ToolCodeExecutor   = "tools.code_executor"
	ToolShellExecutor  = "tools.shell_executor"
	ToolFileOps        = "tools.file_ops"
	ToolDelegate       = "tools.delegate"
	ToolReadSubSession = "tools.read_sub_session"
)

// ---- Wildcard Patterns ----

const (
	WildcardAll    = "*"
	WildcardTools  = "tools.*"
	WildcardHooks  = "hooks.*"
	WildcardSkills = "skills.*"
	WildcardMemory = "memory.*"
	WildcardModules = "modules.*"
)

// ---- User-facing Tool Aliases (for toolNameToCap mapping) ----

const (
	AliasCodeExecutor  = "code_executor"
	AliasShellExecutor = "shell_executor"
	AliasFileOps       = "file_ops"
	AliasTodoList      = "todolist"
	AliasDelegateTo    = "delegate_to"
	AliasReadSubSession = "read_sub_session"
)

// ---- FileMemory Defaults ----

const (
	DefaultMaxIndexLines = 200
	DefaultMaxIndexBytes = 25600 // 25 * 1024
)
