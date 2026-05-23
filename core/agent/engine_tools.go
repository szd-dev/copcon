package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

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

type parsedToolCall struct {
	tc   toolCallInfo
	args map[string]any
}

type toolExecutionResult struct {
	tc     toolCallInfo
	result any
	err    error
}

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

func parseArgs(argsJSON string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return make(map[string]any)
	}
	return args
}

func (e *engineImpl) executeSync(chatCtx iface.ChatContextInterface, toolMgr tool.ToolManager, tc toolCallInfo, args map[string]any, messageID string, stepIndex int, partIndices map[string]int, toolResults map[string]*ToolCallResult) error {
	if partIdx, ok := partIndices[tc.ID]; ok {
		chatCtx.Emit(entity.Event{
			Type: entity.EventPartUpdate,
			Data: entity.PartUpdateData{
				MessageID: messageID,
				StepIndex: stepIndex,
				PartIndex: partIdx,
				PartType:  "tool-call",
				State:     "running",
			},
		})
	}

	e.hookRunner.On(hook.BeforeToolExecute, chatCtx, e.logger, hook.HookExtra{ToolName: &tc.Name, ToolArgs: args})

	chatCtx.SetPartLocator(messageID, stepIndex, partIndices[tc.ID])
	defer chatCtx.ClearPartLocator()

	result, err := toolMgr.Execute(chatCtx, tc.Name, args)

	if err != nil {
		e.hookRunner.On(hook.OnToolError, chatCtx, e.logger, hook.HookExtra{ToolName: &tc.Name, ToolArgs: args, ToolResult: result})
	} else {
		e.hookRunner.On(hook.AfterToolExecute, chatCtx, e.logger, hook.HookExtra{ToolName: &tc.Name, ToolArgs: args, ToolResult: result})
	}

	tr := &ToolCallResult{}
	if err != nil {
		tr.Error = err.Error()
	} else {
		outputJSON, _ := json.Marshal(result)
		tr.Output = string(outputJSON)
	}
	toolResults[tc.ID] = tr

	if partIdx, ok := partIndices[tc.ID]; ok {
		partUpdate := entity.PartUpdateData{
			MessageID: messageID,
			StepIndex: stepIndex,
			PartIndex: partIdx,
			PartType:  "tool-call",
		}
		if tr.Error != "" {
			partUpdate.State = "error"
			partUpdate.Error = tr.Error
		} else {
			partUpdate.State = "complete"
			partUpdate.Output = tr.Output
		}
		chatCtx.Emit(entity.Event{Type: entity.EventPartUpdate, Data: partUpdate})
	}

	var resultJSON []byte
	if err != nil {
		resultJSON, _ = json.Marshal(map[string]any{"error": err.Error()})
	} else {
		resultJSON, _ = json.Marshal(result)
	}
	sessionUUID, parseErr := uuid.Parse(chatCtx.SessionID())
	if parseErr != nil {
		return fmt.Errorf("invalid session ID: %w", parseErr)
	}
	if err := e.messageStore.Add(chatCtx.Context(), &storage.Message{
		SessionID:  sessionUUID,
		Role:       "tool",
		Content:    string(resultJSON),
		ToolCallID: tc.ID,
	}); err != nil {
		return fmt.Errorf("add tool result message: %w", err)
	}

	return nil
}

func (e *engineImpl) executeAsync(chatCtx iface.ChatContextInterface, toolMgr tool.ToolManager, tc toolCallInfo, args map[string]any, messageID string, stepIndex int, partIndices map[string]int) error {
	sessionID := chatCtx.SessionID()
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(chatCtx.Context(), 5*time.Minute)

	e.asyncRegistry.Register(sessionID, tc.ID, tc.Name, cancel)

	chatCtx.Emit(entity.Event{
		Type: entity.EventAsyncToolStarted,
		Data: entity.AsyncToolStartedData{
			MessageID: messageID,
			CallID:    tc.ID,
			ToolName:  tc.Name,
			SessionID: sessionID,
		},
	})

	if partIdx, ok := partIndices[tc.ID]; ok {
		chatCtx.Emit(entity.Event{
			Type: entity.EventPartUpdate,
			Data: entity.PartUpdateData{
				MessageID: messageID,
				StepIndex: stepIndex,
				PartIndex: partIdx,
				PartType:  "tool-call",
				State:     "running",
			},
		})
	}

	e.hookRunner.On(hook.BeforeToolExecute, chatCtx, e.logger, hook.HookExtra{ToolName: &tc.Name, ToolArgs: args})

	go func() {
		defer e.asyncRegistry.Unregister(tc.ID)
		defer cancel()

		if err := e.concurrencySem.Acquire(ctx, 1); err != nil {
			e.asyncRegistry.Fail(tc.ID, err.Error())
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolFailed,
				Data: entity.AsyncToolFailedData{
					MessageID: messageID,
					CallID:    tc.ID,
					ToolName:  tc.Name,
					Error:     err.Error(),
					Duration:  time.Since(startTime).Milliseconds(),
				},
			})
			if partIdx, ok := partIndices[tc.ID]; ok {
				chatCtx.Emit(entity.Event{
					Type: entity.EventPartUpdate,
					Data: entity.PartUpdateData{
						MessageID: messageID,
						StepIndex: stepIndex,
						PartIndex: partIdx,
						PartType:  "tool-call",
						State:     "error",
						Error:     err.Error(),
					},
				})
			}
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
						MessageID: messageID,
						CallID:    tc.ID,
						ToolName:  tc.Name,
						Error:     errMsg,
						Duration:  time.Since(startTime).Milliseconds(),
					},
				})
				if partIdx, ok := partIndices[tc.ID]; ok {
					chatCtx.Emit(entity.Event{
						Type: entity.EventPartUpdate,
						Data: entity.PartUpdateData{
							MessageID: messageID,
							StepIndex: stepIndex,
							PartIndex: partIdx,
							PartType:  "tool-call",
							State:     "error",
							Error:     errMsg,
						},
					})
				}
			}
		}()

		result, err := toolMgr.Execute(chatCtx, tc.Name, args)

		if err != nil {
			e.asyncRegistry.Fail(tc.ID, err.Error())
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolFailed,
				Data: entity.AsyncToolFailedData{
					MessageID: messageID,
					CallID:    tc.ID,
					ToolName:  tc.Name,
					Error:     err.Error(),
					Duration:  time.Since(startTime).Milliseconds(),
				},
			})
			if partIdx, ok := partIndices[tc.ID]; ok {
				chatCtx.Emit(entity.Event{
					Type: entity.EventPartUpdate,
					Data: entity.PartUpdateData{
						MessageID: messageID,
						StepIndex: stepIndex,
						PartIndex: partIdx,
						PartType:  "tool-call",
						State:     "error",
						Error:     err.Error(),
					},
				})
			}

			resultJSON, _ := json.Marshal(map[string]any{"error": err.Error()})
			sessionUUID, _ := uuid.Parse(sessionID)
			if addErr := e.messageStore.Add(chatCtx.Context(), &storage.Message{
				SessionID:  sessionUUID,
				Role:       "tool",
				Content:    string(resultJSON),
				ToolCallID: tc.ID,
			}); addErr != nil {
				e.logger.Error("persist_async_tool_error",
					"session_id", sessionID,
					"tool_name", tc.Name,
					"call_id", tc.ID,
					"error", addErr,
				)
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
			if addErr := e.sessionStore.AppendMetadata(chatCtx.Context(), sessionUUID, "async_completion_pending", pendingEvent); addErr != nil {
				e.logger.Error("record_async_completion_pending_error",
					"session_id", sessionID,
					"tool_name", tc.Name,
					"call_id", tc.ID,
					"status", "failed",
					"error", addErr,
				)
			}
		} else {
			e.asyncRegistry.Complete(tc.ID, result)
			chatCtx.Emit(entity.Event{
				Type: entity.EventAsyncToolComplete,
				Data: entity.AsyncToolCompleteData{
					MessageID: messageID,
					CallID:    tc.ID,
					ToolName:  tc.Name,
					Result:    result,
					Duration:  time.Since(startTime).Milliseconds(),
				},
			})
			if partIdx, ok := partIndices[tc.ID]; ok {
				outputJSON, _ := json.Marshal(result)
				chatCtx.Emit(entity.Event{
					Type: entity.EventPartUpdate,
					Data: entity.PartUpdateData{
						MessageID: messageID,
						StepIndex: stepIndex,
						PartIndex: partIdx,
						PartType:  "tool-call",
						State:     "complete",
						Output:    string(outputJSON),
					},
				})
			}

			resultJSON, _ := json.Marshal(result)
			sessionUUID, _ := uuid.Parse(sessionID)
			if addErr := e.messageStore.Add(chatCtx.Context(), &storage.Message{
				SessionID:  sessionUUID,
				Role:       "tool",
				Content:    string(resultJSON),
				ToolCallID: tc.ID,
			}); addErr != nil {
				e.logger.Error("persist_async_tool_result_error",
					"session_id", sessionID,
					"tool_name", tc.Name,
					"call_id", tc.ID,
					"error", addErr,
				)
			}

			pendingEvent := map[string]any{
				"id":           uuid.New().String(),
				"call_id":      tc.ID,
				"tool_name":    tc.Name,
				"session_id":   sessionID,
				"completed_at": time.Now().Format(time.RFC3339),
				"status":       "completed",
			}
			if addErr := e.sessionStore.AppendMetadata(chatCtx.Context(), sessionUUID, "async_completion_pending", pendingEvent); addErr != nil {
				e.logger.Error("record_async_completion_pending_error",
					"session_id", sessionID,
					"tool_name", tc.Name,
					"call_id", tc.ID,
					"status", "completed",
					"error", addErr,
				)
			}
		}
	}()

	return nil
}

func (e *engineImpl) executeConcurrent(
	chatCtx iface.ChatContextInterface,
	toolMgr tool.ToolManager,
	toolCalls []parsedToolCall,
	messageID string,
	stepIndex int,
	partIndices map[string]int,
	toolResults map[string]*ToolCallResult,
) error {
	if len(toolCalls) == 0 {
		return nil
	}

	var (
		results = make([]toolExecutionResult, len(toolCalls))
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	for i, p := range toolCalls {
		wg.Add(1)
		go func(idx int, p parsedToolCall) {
			defer wg.Done()

			if err := e.concurrencySem.Acquire(chatCtx.Context(), 1); err != nil {
				mu.Lock()
				results[idx] = toolExecutionResult{
					tc:  p.tc,
					err: fmt.Errorf("semaphore acquire: %w", err),
				}
				mu.Unlock()
				return
			}
			defer e.concurrencySem.Release(1)

			if partIdx, ok := partIndices[p.tc.ID]; ok {
				chatCtx.Emit(entity.Event{
					Type: entity.EventPartUpdate,
					Data: entity.PartUpdateData{
						MessageID: messageID,
						StepIndex: stepIndex,
						PartIndex: partIdx,
						PartType:  "tool-call",
						State:     "running",
					},
				})
			}

			e.hookRunner.On(hook.BeforeToolExecute, chatCtx, e.logger, hook.HookExtra{ToolName: &p.tc.Name, ToolArgs: p.args})

			execResult, execErr := toolMgr.Execute(chatCtx, p.tc.Name, p.args)

			if execErr != nil {
				e.hookRunner.On(hook.OnToolError, chatCtx, e.logger, hook.HookExtra{ToolName: &p.tc.Name, ToolArgs: p.args, ToolResult: execResult})
			} else {
				e.hookRunner.On(hook.AfterToolExecute, chatCtx, e.logger, hook.HookExtra{ToolName: &p.tc.Name, ToolArgs: p.args, ToolResult: execResult})
			}

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

			tr := &ToolCallResult{}
			if execErr != nil {
				tr.Error = execErr.Error()
			} else {
				outputJSON, _ := json.Marshal(execResult)
				tr.Output = string(outputJSON)
			}
			toolResults[p.tc.ID] = tr

			mu.Unlock()

			if partIdx, ok := partIndices[p.tc.ID]; ok {
				partUpdate := entity.PartUpdateData{
					MessageID: messageID,
					StepIndex: stepIndex,
					PartIndex: partIdx,
					PartType:  "tool-call",
				}
				if execErr != nil {
					partUpdate.State = "error"
					partUpdate.Error = execErr.Error()
				} else {
					partUpdate.State = "complete"
					outputJSON, _ := json.Marshal(execResult)
					partUpdate.Output = string(outputJSON)
				}
				chatCtx.Emit(entity.Event{Type: entity.EventPartUpdate, Data: partUpdate})
			}
		}(i, p)
	}

	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return results[i].tc.ID < results[j].tc.ID
	})

	sessionUUID, parseErr := uuid.Parse(chatCtx.SessionID())
	if parseErr != nil {
		return fmt.Errorf("invalid session ID: %w", parseErr)
	}

	for _, r := range results {
		resultJSON, _ := json.Marshal(r.result)
		if err := e.messageStore.Add(chatCtx.Context(), &storage.Message{
			SessionID:  sessionUUID,
			Role:       "tool",
			Content:    string(resultJSON),
			ToolCallID: r.tc.ID,
		}); err != nil {
			return fmt.Errorf("add tool result message for %s: %w", r.tc.ID, err)
		}
	}

	return nil
}

func (e *engineImpl) handleToolCalls(
	chatCtx iface.ChatContextInterface,
	toolMgr tool.ToolManager,
	result *StreamResult,
	persistedMsgUUID *string,
	accumulatedParts *[]storage.Part,
	accumulatedToolCalls *[]storage.ToolCall,
) (bool, error) {
	if len(result.ToolCalls) > 0 {
		partIndex := 0
		if result.ReasoningContent != "" {
			partIndex++
		}
		if result.Content != "" || len(result.ToolCalls) == 0 {
			partIndex++
		}

		stepIndex := result.StepIndex
		toolCallPartIndices := make(map[string]int)
		for _, tc := range result.ToolCalls {
			chatCtx.Emit(entity.Event{
				Type: entity.EventPartCreate,
				Data: entity.PartCreateData{
					MessageID:  result.MessageID,
					StepIndex:  stepIndex,
					PartIndex:  partIndex,
					PartType:   "tool-call",
					State:      "pending",
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Args:       tc.Arguments,
				},
			})
			toolCallPartIndices[tc.ID] = partIndex
			partIndex++
		}

		toolResults := make(map[string]*ToolCallResult)

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
			if err := e.executeSync(chatCtx, toolMgr, tc, args, result.MessageID, stepIndex, toolCallPartIndices, toolResults); err != nil {
				return false, fmt.Errorf("execute sync tool call: %w", err)
			}
			result.ToolResults = toolResults
			e.checkpointToolResult(chatCtx, result, persistedMsgUUID, accumulatedParts)
		}

		if len(concurrentToolCalls) > 0 {
			if err := e.executeConcurrent(chatCtx, toolMgr, concurrentToolCalls, result.MessageID, stepIndex, toolCallPartIndices, toolResults); err != nil {
				return false, fmt.Errorf("execute concurrent tool calls: %w", err)
			}
			result.ToolResults = toolResults
			e.checkpointToolResult(chatCtx, result, persistedMsgUUID, accumulatedParts)
		}

		for _, tc := range asyncToolCalls {
			args := parseArgs(tc.Arguments)
			_, args = parseExecutionMode(args)
			if err := e.executeAsync(chatCtx, toolMgr, tc, args, result.MessageID, stepIndex, toolCallPartIndices); err != nil {
				return false, fmt.Errorf("execute async tool call: %w", err)
			}
		}

		result.ToolResults = toolResults

		if err := e.persistMessage(chatCtx, result, false, persistedMsgUUID, accumulatedParts, accumulatedToolCalls); err != nil {
			return false, err
		}

		return true, nil
	}

	if err := e.persistMessage(chatCtx, result, true, persistedMsgUUID, accumulatedParts, accumulatedToolCalls); err != nil {
		return false, err
	}

	chatCtx.Emit(entity.Event{
		Type: entity.EventMessageDone,
		Data: entity.MessageDoneData{MessageID: result.MessageID},
	})

	return false, nil
}

func (e *engineImpl) convertToolCalls(toolCalls []toolCallInfo) []storage.ToolCall {
	result := make([]storage.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = storage.ToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: storage.FunctionCall{
				Name:      tc.Name,
				Arguments: tc.Arguments,
			},
		}
	}
	return result
}
