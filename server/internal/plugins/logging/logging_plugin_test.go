package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

// stubChatContext is a minimal ChatContextInterface for tests.
type stubChatContext struct {
	ctx       context.Context
	sessionID string
	agentID   string
}

func (s *stubChatContext) Context() context.Context                  { return s.ctx }
func (s *stubChatContext) SessionID() string                         { return s.sessionID }
func (s *stubChatContext) AgentID() string                           { return s.agentID }
func (s *stubChatContext) Events() <-chan entity.Event               { return nil }
func (s *stubChatContext) Emit(_ entity.Event)                       {}
func (s *stubChatContext) Close()                                    {}
func (s *stubChatContext) Closed() <-chan struct{}                   { ch := make(chan struct{}); close(ch); return ch }
func (s *stubChatContext) Depth() int                                { return 0 }
func (s *stubChatContext) Subscribe(int64) (*iface.Subscriber, bool) { return nil, false }
func (s *stubChatContext) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, fmt.Errorf("stub: RequestInput not implemented")
}
func (s *stubChatContext) ResolveInput(interruptID string, resp *iface.InputResponse) error {
	return iface.ErrInterruptNotFound
}
func (s *stubChatContext) PendingInputs() []iface.InputRequest {
	return nil
}
func (s *stubChatContext) SetPartLocator(messageID string, stepIndex, partIndex int) {}
func (s *stubChatContext) ClearPartLocator()                                         {}

// newTestLogger creates a slog.Logger that writes to a bytes.Buffer
// so tests can inspect the log output.
func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)
	return logger, buf
}

// makeMessages creates a slice of MessageForLLM for testing.
func makeMessages(count int) []entity.MessageForLLM {
	msgs := make([]entity.MessageForLLM, count)
	for i := range count {
		msgs[i] = entity.MessageForLLM{
			Role:    "user",
			Content: "test message " + string(rune('0'+i)),
		}
	}
	return msgs
}

func TestLoggingPlugin_Name(t *testing.T) {
	p := NewLoggingPlugin()
	assert.Equal(t, "logging", p.Name())
}

func TestLoggingPlugin_Points(t *testing.T) {
	p := NewLoggingPlugin()
	expected := []hook.HookPoint{
		hook.BeforeLLMCall,
		hook.AfterLLMCall,
		hook.BeforeToolExecute,
		hook.AfterToolExecute,
	}
	assert.Equal(t, expected, p.Points())
}

func TestLoggingPlugin_Priority(t *testing.T) {
	p := NewLoggingPlugin()
	assert.Equal(t, 200, p.Priority())
}

func TestLoggingPlugin_BeforeLLMCall(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s1",
		agentID:   "a1",
	}
	msgs := makeMessages(5)

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s1",
		AgentID:      "a1",
		Messages:     &msgs,
		Logger:       logger,
		CurrentPoint: hook.BeforeLLMCall,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "before_llm_call")
	assert.Contains(t, output, "session_id=s1")
	assert.Contains(t, output, "agent_id=a1")
	assert.Contains(t, output, "message_count=5")

	// Verify no message content leaked
	assert.NotContains(t, output, "test message")
}

func TestLoggingPlugin_BeforeLLMCall_NilMessages(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s2",
		agentID:   "a2",
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s2",
		AgentID:      "a2",
		Messages:     nil,
		Logger:       logger,
		CurrentPoint: hook.BeforeLLMCall,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "before_llm_call")
	assert.Contains(t, output, "message_count=0")
}

func TestLoggingPlugin_AfterLLMCall(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s3",
		agentID:   "a3",
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s3",
		AgentID:      "a3",
		Logger:       logger,
		CurrentPoint: hook.AfterLLMCall,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "after_llm_call")
	assert.Contains(t, output, "session_id=s3")
	assert.Contains(t, output, "agent_id=a3")
}

func TestLoggingPlugin_BeforeToolExecute(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s4",
		agentID:   "a4",
	}

	toolArgs := map[string]any{
		"file": "/tmp/test.txt",
		"mode": "read",
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s4",
		AgentID:      "a4",
		ToolName:     "read_file",
		ToolArgs:     toolArgs,
		Logger:       logger,
		CurrentPoint: hook.BeforeToolExecute,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "before_tool_execute")
	assert.Contains(t, output, "session_id=s4")
	assert.Contains(t, output, "agent_id=a4")
	assert.Contains(t, output, "tool_name=read_file")
	assert.Contains(t, output, "tool_args=")
}

func TestLoggingPlugin_AfterToolExecute_Success(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s5",
		agentID:   "a5",
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:   stubCtx,
		SessionID: "s5",
		AgentID:   "a5",
		ToolName:  "write_file",
		ToolResult: &tool.ToolResult{
			Success: true,
			Data:    map[string]any{"path": "/tmp/out.txt"},
		},
		Logger:       logger,
		CurrentPoint: hook.AfterToolExecute,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "after_tool_execute")
	assert.Contains(t, output, "session_id=s5")
	assert.Contains(t, output, "agent_id=a5")
	assert.Contains(t, output, "tool_name=write_file")
	assert.Contains(t, output, "success=true")

	// Verify no result data leaked
	assert.NotContains(t, output, "/tmp/out.txt")
}

func TestLoggingPlugin_AfterToolExecute_Error(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s6",
		agentID:   "a6",
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:   stubCtx,
		SessionID: "s6",
		AgentID:   "a6",
		ToolName:  "delete_file",
		ToolResult: &tool.ToolResult{
			Success: false,
			Error:   "permission denied: /etc/hosts",
		},
		Logger:       logger,
		CurrentPoint: hook.AfterToolExecute,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "after_tool_execute")
	assert.Contains(t, output, "tool_name=delete_file")
	assert.Contains(t, output, "success=false")
	// slog.TextHandler quotes values containing spaces/colons
	assert.Contains(t, output, "error=")
}

func TestLoggingPlugin_AfterToolExecute_NilResult(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s7",
		agentID:   "a7",
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s7",
		AgentID:      "a7",
		ToolName:     "unknown_tool",
		ToolResult:   nil,
		Logger:       logger,
		CurrentPoint: hook.AfterToolExecute,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "after_tool_execute")
	assert.Contains(t, output, "success=false")
}

func TestLoggingPlugin_ToolArgsTruncation(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s8",
		agentID:   "a8",
	}

	// Create args that will be > 500 chars when JSON-serialized
	longString := strings.Repeat("x", 600)
	toolArgs := map[string]any{
		"data": longString,
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s8",
		AgentID:      "a8",
		ToolName:     "process_data",
		ToolArgs:     toolArgs,
		Logger:       logger,
		CurrentPoint: hook.BeforeToolExecute,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "before_tool_execute")
	assert.Contains(t, output, "tool_name=process_data")

	// Verify args are not logged in full — output should not contain
	// the complete 600-char JSON string (around 488 x's survive after truncation)
	assert.NotContains(t, output, strings.Repeat("x", 550))
}

func TestLoggingPlugin_ToolArgsNil(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s9",
		agentID:   "a9",
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s9",
		AgentID:      "a9",
		ToolName:     "dummy",
		ToolArgs:     nil,
		Logger:       logger,
		CurrentPoint: hook.BeforeToolExecute,
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "tool_args={}")
}

func TestLoggingPlugin_NoContentLeakage(t *testing.T) {
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s10",
		agentID:   "a10",
	}

	msgs := makeMessages(3)
	msgs[0].Content = "SECRET_API_KEY=abc123"
	msgs[1].Content = "password: hunter2"
	msgs[2].Content = "user login: admin"

	t.Run("BeforeLLMCall_no_content", func(t *testing.T) {
		buf.Reset()
		p := NewLoggingPlugin()
		err := p.Execute(&hook.HookContext{
			ChatCtx:      stubCtx,
			SessionID:    "s10",
			AgentID:      "a10",
			Messages:     &msgs,
			Logger:       logger,
			CurrentPoint: hook.BeforeLLMCall,
		})
		require.NoError(t, err)

		output := buf.String()
		assert.NotContains(t, output, "SECRET_API_KEY")
		assert.NotContains(t, output, "hunter2")
		assert.NotContains(t, output, "admin")
		assert.NotContains(t, output, "abc123")
	})

	t.Run("AfterToolExecute_no_data", func(t *testing.T) {
		buf.Reset()
		p := NewLoggingPlugin()
		err := p.Execute(&hook.HookContext{
			ChatCtx:   stubCtx,
			SessionID: "s10",
			AgentID:   "a10",
			ToolName:  "secret_tool",
			ToolResult: &tool.ToolResult{
				Success: true,
				Data:    map[string]any{"password": "hunter2", "key": "abc123"},
			},
			Logger:       logger,
			CurrentPoint: hook.AfterToolExecute,
		})
		require.NoError(t, err)

		output := buf.String()
		assert.NotContains(t, output, "hunter2")
		assert.NotContains(t, output, "abc123")
		assert.NotContains(t, output, "password")
	})
}

func TestLoggingPlugin_ImplementsHookInterface(t *testing.T) {
	// Compile-time check: verify LoggingPlugin satisfies hook.Hook
	var _ hook.Hook = (*LoggingPlugin)(nil)

	// Runtime check
	p := NewLoggingPlugin()
	assert.Implements(t, (*hook.Hook)(nil), p)
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short", "hello", 100, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello wo..."},
		{"empty", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncateArgs(t *testing.T) {
	t.Run("nil_args", func(t *testing.T) {
		got := truncateArgs(nil, 500)
		assert.Equal(t, "{}", got)
	})

	t.Run("small_args", func(t *testing.T) {
		args := map[string]any{"key": "value"}
		got := truncateArgs(args, 500)
		var parsed map[string]any
		err := json.Unmarshal([]byte(got), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "value", parsed["key"])
	})

	t.Run("long_args_truncated", func(t *testing.T) {
		longVal := strings.Repeat("x", 600)
		args := map[string]any{"data": longVal}
		got := truncateArgs(args, 100)
		assert.LessOrEqual(t, len(got), 103)
		assert.True(t, strings.HasSuffix(got, "..."),
			"should end with '...', got: %s", got[:min(len(got), 50)])
	})
}

func TestLoggingPlugin_WrongHookPoint(t *testing.T) {
	// Verify the plugin handles unexpected hook points gracefully
	logger, buf := newTestLogger()
	stubCtx := &stubChatContext{
		ctx:       context.Background(),
		sessionID: "s11",
		agentID:   "a11",
	}

	p := NewLoggingPlugin()
	err := p.Execute(&hook.HookContext{
		ChatCtx:      stubCtx,
		SessionID:    "s11",
		AgentID:      "a11",
		Logger:       logger,
		CurrentPoint: hook.OnSystemPrompt, // not in Points()
	})
	require.NoError(t, err)

	// Should not log anything for unregistered points
	output := buf.String()
	assert.Empty(t, output, "should not log for unregistered hook points")
}

// Note: stubChatContext intentionally matches the iface.ChatContextInterface
// pattern used throughout the codebase. The `Emit` method signature issue is
// a pre-existing problem in the `iface` package, not specific to this test.
