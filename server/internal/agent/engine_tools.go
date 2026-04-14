package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
)

// ExecutionMode defines how a tool should be executed.
type ExecutionMode string

const (
	ExecutionModeSync       ExecutionMode = "sync"
	ExecutionModeConcurrent ExecutionMode = "concurrent"
	ExecutionModeAsync      ExecutionMode = "async"
)

type toolCallInfo struct {
	MessageID string
	ID        string
	Name      string
	Arguments string
}

// parsedToolCall holds a tool call with its parsed arguments (execution_mode removed).
type parsedToolCall struct {
	tc   toolCallInfo
	args map[string]any
}

// toolExecutionResult holds the result of a single tool execution.
type toolExecutionResult struct {
	tc     toolCallInfo
	result any
	err    error
}

// parseExecutionMode extracts the execution_mode from tool arguments.
// It returns the mode (defaulting to sync) and the args with execution_mode removed.
func parseExecutionMode(args map[string]any) (ExecutionMode, map[string]any) {
	mode := ExecutionModeSync
	if val, ok := args["execution_mode"]; ok {
		if str, ok := val.(string); ok {
			switch str {
			case "sync", "concurrent", "async":
				mode = ExecutionMode(str)
			}
		}
		delete(args, "execution_mode")
	}
	return mode, args
}

// parseArgs parses JSON arguments string into a map.
func parseArgs(argsJSON string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return make(map[string]any)
	}
	return args
}

// executeSync executes a tool call synchronously.
func (e *AgentEngine) executeSync(chatCtx iface.ChatContextInterface, toolMgr tool.ToolManager, tc toolCallInfo, args map[string]any) error {
	chatCtx.Emit(entity.Event{
		Type: entity.EventToolCall,
		Data: entity.ToolCallData{
			ToolName:  tc.Name,
			Args:      args,
			ID:        tc.ID,
			MessageID: tc.MessageID,
		},
	})

	result, err := toolMgr.Execute(chatCtx, tc.Name, args)

	var resultData entity.ToolResultData
	if err != nil {
		resultData = entity.ToolResultData{
			ToolName:  tc.Name,
			Result:    map[string]any{"error": err.Error()},
			ID:        tc.ID,
			MessageID: tc.MessageID,
		}
	} else {
		resultData = entity.ToolResultData{
			ToolName:  tc.Name,
			Result:    result,
			ID:        tc.ID,
			MessageID: tc.MessageID,
		}
	}

	chatCtx.Emit(entity.Event{Type: entity.EventToolResult, Data: resultData})

	resultJSON, _ := json.Marshal(resultData.Result)
	if err := e.contextMgr.AddMessage(chatCtx, &session.Message{
		Role:       "tool",
		Content:    string(resultJSON),
		ToolCallID: tc.ID,
	}); err != nil {
		return fmt.Errorf("add tool result message: %w", err)
	}

	return nil
}

// executeAsync executes a tool call asynchronously in a goroutine.
func (e *AgentEngine) executeAsync(chatCtx iface.ChatContextInterface, toolMgr tool.ToolManager, tc toolCallInfo, args map[string]any) error {
	sessionID := chatCtx.SessionID()
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(chatCtx.Context(), 5*time.Minute)

	e.asyncRegistry.Register(sessionID, tc.ID, tc.Name, cancel)

	chatCtx.Emit(entity.Event{
		Type: entity.EventAsyncToolStarted,
		Data: entity.AsyncToolStartedData{
			CallID:    tc.ID,
			ToolName:  tc.Name,
			SessionID: sessionID,
		},
	})

	go func() {
		defer e.asyncRegistry.Unregister(tc.ID)
		defer cancel()

		if err := e.concurrencySem.Acquire(ctx, 1); err != nil {
			e.asyncRegistry.Fail(tc.ID, err.Error())
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolFailed,
				Data: entity.AsyncToolFailedData{
					CallID:   tc.ID,
					ToolName: tc.Name,
					Error:    err.Error(),
					Duration: time.Since(startTime).Milliseconds(),
				},
			})
			return
		}
		defer e.concurrencySem.Release(1)

		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				errMsg := fmt.Sprintf("panic: %v\n%s", r, stack)
				e.asyncRegistry.Fail(tc.ID, errMsg)
				chatCtx.Emit(entity.Event{
					Type: entity.EventAsyncToolFailed,
					Data: entity.AsyncToolFailedData{
						CallID:   tc.ID,
						ToolName: tc.Name,
						Error:    errMsg,
						Duration: time.Since(startTime).Milliseconds(),
					},
				})
			}
		}()

		result, err := toolMgr.Execute(chatCtx, tc.Name, args)

		if err != nil {
			e.asyncRegistry.Fail(tc.ID, err.Error())
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolFailed,
				Data: entity.AsyncToolFailedData{
					CallID:   tc.ID,
					ToolName: tc.Name,
					Error:    err.Error(),
					Duration: time.Since(startTime).Milliseconds(),
				},
			})

			resultJSON, _ := json.Marshal(map[string]any{"error": err.Error()})
			if addErr := e.contextMgr.AddMessage(chatCtx, &session.Message{
				Role:       "tool",
				Content:    string(resultJSON),
				ToolCallID: tc.ID,
			}); addErr != nil {
				log.Printf("failed to persist async tool error: %v", addErr)
			}

			pendingEvent := map[string]any{
				"id":           uuid.New().String(),
				"call_id":      tc.ID,
				"tool_name":    tc.Name,
				"session_id":   sessionID,
				"completed_at": time.Now().Format(time.RFC3339),
				"status":       "failed",
				"error":        err.Error(),
			}
			if addErr := e.sessionMgr.AddAsyncCompletionPending(chatCtx, pendingEvent); addErr != nil {
				log.Printf("failed to record async completion pending: %v", addErr)
			}
		} else {
			e.asyncRegistry.Complete(tc.ID, result)
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolComplete,
				Data: entity.AsyncToolCompleteData{
					CallID:   tc.ID,
					ToolName: tc.Name,
					Result:   result,
					Duration: time.Since(startTime).Milliseconds(),
				},
			})

			resultJSON, _ := json.Marshal(result)
			if addErr := e.contextMgr.AddMessage(chatCtx, &session.Message{
				Role:       "tool",
				Content:    string(resultJSON),
				ToolCallID: tc.ID,
			}); addErr != nil {
				log.Printf("failed to persist async tool result: %v", addErr)
			}

			pendingEvent := map[string]any{
				"id":           uuid.New().String(),
				"call_id":      tc.ID,
				"tool_name":    tc.Name,
				"session_id":   sessionID,
				"completed_at": time.Now().Format(time.RFC3339),
				"status":       "completed",
			}
			if addErr := e.sessionMgr.AddAsyncCompletionPending(chatCtx, pendingEvent); addErr != nil {
				log.Printf("failed to record async completion pending: %v", addErr)
			}
		}
	}()

	return nil
}

// executeConcurrent executes multiple tool calls concurrently with semaphore limiting.
// It uses a semaphore to limit concurrency to 5 and collects results safely with mutex protection.
// One failure does not stop other executions - all tools run to completion.
// Results are sorted by tool call ID and persisted in order.
func (e *AgentEngine) executeConcurrent(
	chatCtx iface.ChatContextInterface,
	toolMgr tool.ToolManager,
	toolCalls []parsedToolCall,
) error {
	if len(toolCalls) == 0 {
		return nil
	}

	var (
		results = make([]toolExecutionResult, len(toolCalls))
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	// Launch goroutines for each tool call
	for i, p := range toolCalls {
		wg.Add(1)
		go func(idx int, p parsedToolCall) {
			defer wg.Done()

			// Acquire semaphore (blocks if 5 concurrent executions are already running)
			if err := e.concurrencySem.Acquire(chatCtx.Context(), 1); err != nil {
				// Context cancelled or semaphore error
				mu.Lock()
				results[idx] = toolExecutionResult{
					tc:  p.tc,
					err: fmt.Errorf("semaphore acquire: %w", err),
				}
				mu.Unlock()
				return
			}
			defer e.concurrencySem.Release(1)

			// Emit tool call event
			chatCtx.Emit(entity.Event{
				Type: entity.EventToolCall,
				Data: entity.ToolCallData{
					ToolName:  p.tc.Name,
					Args:      p.args,
					ID:        p.tc.ID,
					MessageID: p.tc.MessageID,
				},
			})

			// Execute tool
			execResult, execErr := toolMgr.Execute(chatCtx, p.tc.Name, p.args)

			var result any
			if execErr != nil {
				result = map[string]any{"error": execErr.Error()}
			} else {
				result = execResult
			}

			mu.Lock()
			results[idx] = toolExecutionResult{
				tc:     p.tc,
				result: result,
				err:    execErr,
			}
			mu.Unlock()

			chatCtx.Emit(entity.Event{
				Type: entity.EventToolResult,
				Data: entity.ToolResultData{
					ToolName:  p.tc.Name,
					Result:    result,
					ID:        p.tc.ID,
					MessageID: p.tc.MessageID,
				},
			})
		}(i, p)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Sort results by tool call ID for consistent ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].tc.ID < results[j].tc.ID
	})

	// Persist all results in order
	for _, r := range results {
		resultJSON, _ := json.Marshal(r.result)
		if err := e.contextMgr.AddMessage(chatCtx, &session.Message{
			Role:       "tool",
			Content:    string(resultJSON),
			ToolCallID: r.tc.ID,
		}); err != nil {
			return fmt.Errorf("add tool result message for %s: %w", r.tc.ID, err)
		}
	}

	return nil
}

// handleToolCalls processes tool calls from the streaming result.
// Returns true if the loop should continue (tool calls were executed),
// false if the loop should exit (no tool calls, final message persisted).
func (e *AgentEngine) handleToolCalls(
	chatCtx iface.ChatContextInterface,
	toolMgr tool.ToolManager,
	result *StreamResult,
) (bool, error) {
	if len(result.ToolCalls) > 0 {
		if err := e.persistMessage(chatCtx, result, false); err != nil {
			return false, err
		}

		var (
			syncToolCalls       []toolCallInfo
			concurrentToolCalls []parsedToolCall
			asyncToolCalls      []toolCallInfo
		)

		for _, tc := range result.ToolCalls {
			args := parseArgs(tc.Arguments)
			mode, args := parseExecutionMode(args)

			switch mode {
			case ExecutionModeSync:
				syncToolCalls = append(syncToolCalls, tc)
			case ExecutionModeConcurrent:
				concurrentToolCalls = append(concurrentToolCalls, parsedToolCall{tc: tc, args: args})
			case ExecutionModeAsync:
				asyncToolCalls = append(asyncToolCalls, tc)
			}
		}

		for _, tc := range syncToolCalls {
			args := parseArgs(tc.Arguments)
			_, args = parseExecutionMode(args)
			if err := e.executeSync(chatCtx, toolMgr, tc, args); err != nil {
				return false, fmt.Errorf("execute sync tool call: %w", err)
			}
		}

		if len(concurrentToolCalls) > 0 {
			if err := e.executeConcurrent(chatCtx, toolMgr, concurrentToolCalls); err != nil {
				return false, fmt.Errorf("execute concurrent tool calls: %w", err)
			}
		}

		for _, tc := range asyncToolCalls {
			args := parseArgs(tc.Arguments)
			_, args = parseExecutionMode(args)
			if err := e.executeAsync(chatCtx, toolMgr, tc, args); err != nil {
				return false, fmt.Errorf("execute async tool call: %w", err)
			}
		}

		return true, nil
	}

	if err := e.persistMessage(chatCtx, result, true); err != nil {
		return false, err
	}

	chatCtx.Emit(entity.Event{
		Type: entity.EventDone,
		Data: entity.DoneData{MessageID: result.MessageID},
	})

	return false, nil
}

// convertToolCalls converts internal toolCallInfo slice to session.ToolCall format.
func (e *AgentEngine) convertToolCalls(toolCalls []toolCallInfo) []session.ToolCall {
	result := make([]session.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = session.ToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: session.FunctionCall{
				Name:      tc.Name,
				Arguments: tc.Arguments,
			},
		}
	}
	return result
}
