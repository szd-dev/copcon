// Package hook provides the foundational types and interfaces for the
// core-periphery hook system. Hooks allow external code to intercept
// and influence key lifecycle events in the agent engine without
// modifying core logic.
//
// The hook system is central to the periphery architecture: hooks are
// registered from the outside and executed at well-defined points in
// the engine's request processing pipeline.
package hook

import (
	"log/slog"

	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/tool"
)

// HookPoint identifies a specific point in the agent engine lifecycle
// where a hook may be executed. Each constant represents a distinct
// interception point with a unique semantic meaning.
type HookPoint string

const (
	// BeforeContextBuild fires before the context window is assembled
	// for an LLM request. Hooks may modify the system prompt at this
	// stage.
	BeforeContextBuild HookPoint = "before_context_build"

	// AfterContextBuild fires after the context window has been built
	// but before it is sent to the LLM. Hooks may inspect or modify
	// the assembled messages.
	AfterContextBuild HookPoint = "after_context_build"

	// OnSystemPrompt fires when the system prompt is being resolved.
	// Hooks may replace or augment the system prompt text.
	OnSystemPrompt HookPoint = "on_system_prompt"

	// OnMessagePersist fires when a message is about to be persisted
	// to storage. Hooks may transform or filter the message before
	// it is written.
	OnMessagePersist HookPoint = "on_message_persist"

	// BeforeToolExecute fires immediately before a tool is invoked.
	// Hooks may modify the tool arguments or abort execution.
	BeforeToolExecute HookPoint = "before_tool_execute"

	// AfterToolExecute fires after a tool has completed successfully.
	// Hooks may transform or augment the tool result.
	AfterToolExecute HookPoint = "after_tool_execute"

	// OnToolError fires when a tool execution fails. Hooks may
	// inspect the error, retry, or provide a fallback result.
	OnToolError HookPoint = "on_tool_error"

	// BeforeLLMCall fires before an LLM API request is dispatched.
	// Hooks may modify the request parameters.
	BeforeLLMCall HookPoint = "before_llm_call"

	// AfterLLMCall fires after an LLM API response is received.
	// Hooks may inspect or transform the response before it is
	// processed by the engine.
	AfterLLMCall HookPoint = "after_llm_call"

	// OnSessionResolve fires when a session ID is being resolved
	// (e.g., during session creation or lookup). Hooks may provide
	// an alternative session resolution.
	OnSessionResolve HookPoint = "on_session_resolve"
)

// HookContext carries all contextual data available at a hook
// execution point. Not all fields are populated for every hook
// point — the specific fields available depend on the HookPoint
// at which the hook is executing.
//
// Pointer fields (*string, *[]MessageForLLM, *tool.ToolResult) are
// used for values that hooks may need to mutate. A hook may set
// these fields to change the behavior of the engine downstream.
type HookContext struct {
	// ChatCtx is the unified chat context providing session identity
	// and event streaming capabilities. It is always populated.
	ChatCtx iface.ChatContextInterface

	// SessionID is the current session identifier. Always populated.
	SessionID string

	// AgentID is the identifier of the agent handling the current
	// request. Always populated.
	AgentID string

	// SystemPrompt is a pointer to the resolved system prompt text.
	// Hooks at OnSystemPrompt may replace the prompt by setting
	// *SystemPrompt. Populated for: OnSystemPrompt, BeforeContextBuild.
	SystemPrompt *string

	// Messages is a pointer to the assembled message list for the
	// current LLM call. Hooks at AfterContextBuild or BeforeLLMCall
	// may modify the message list. Populated for: AfterContextBuild,
	// BeforeLLMCall, AfterLLMCall.
	Messages *[]chat_context.MessageForLLM

	// ToolName is the name of the tool being executed. Populated
	// for: BeforeToolExecute, AfterToolExecute, OnToolError.
	ToolName string

	// ToolArgs is the arguments passed to the tool invocation.
	// Hooks at BeforeToolExecute may modify these arguments.
	// Populated for: BeforeToolExecute, AfterToolExecute, OnToolError.
	ToolArgs map[string]any

	// ToolResult is a pointer to the result of a tool execution.
	// Hooks at AfterToolExecute may transform or augment the result.
	// Hooks at OnToolError may set this field to provide a fallback
	// result. Populated for: AfterToolExecute, OnToolError.
	ToolResult *tool.ToolResult

	// Logger is a structured logger scoped to the current request.
	// Always populated.
	Logger *slog.Logger

	// CurrentPoint is the hook point at which this context is
	// being dispatched. Always populated.
	CurrentPoint HookPoint
}

// Hook defines a piece of logic that executes at one or more points
// in the agent engine lifecycle. Implementations must be safe for
// concurrent use if they modify shared state.
//
// Priority determines execution order: lower numbers run first.
// The default priority is 100.
type Hook interface {
	// Name returns a human-readable identifier for this hook.
	// It is used for logging and debugging purposes.
	Name() string

	// Points returns the set of HookPoint values at which this
	// hook should be executed. A hook may register for multiple
	// points.
	Points() []HookPoint

	// Priority returns the execution order of this hook relative
	// to other hooks registered at the same point. Lower values
	// execute first. The default priority is 100.
	Priority() int

	// Execute is called when the engine reaches a hook point that
	// this hook has registered for. The hook receives a HookContext
	// with fields populated according to the CurrentPoint.
	//
	// Returning a non-nil error will NOT halt the pipeline — errors
	// are logged and the pipeline continues. To abort execution,
	// a hook should return a sentinel error that the HookRunner
	// recognizes.
	Execute(ctx *HookContext) error
}
