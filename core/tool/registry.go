package tool

import (
	"context"
	"errors"
	"sync"
	"time"
)

// AsyncToolStatus represents the lifecycle state of an async tool execution
type AsyncToolStatus string

const (
	StatusRunning   AsyncToolStatus = "running"
	StatusCompleted AsyncToolStatus = "completed"
	StatusFailed    AsyncToolStatus = "failed"
	StatusCancelled AsyncToolStatus = "cancelled"
)

// AsyncToolState tracks the state of an asynchronous tool execution
type AsyncToolState struct {
	CallID     string
	ToolName   string
	Status     AsyncToolStatus
	StartTime  time.Time
	EndTime    time.Time
	Result     any
	Error      string
	SessionID  string
	CancelFunc context.CancelFunc
}

// AsyncToolTracker tracks async tool executions. All methods must be goroutine-safe.
type AsyncToolTracker interface {
	Register(sessionID, callID, toolName string, cancelFunc context.CancelFunc)
	Unregister(callID string)
	Complete(callID string, result any)
	Fail(callID string, errMsg string)
	GetStatus(callID string) (*AsyncToolState, error)
	Cancel(callID string) bool
	CancelSession(sessionID string) int
	ListBySession(sessionID string) []*AsyncToolState
}

// Compile-time check that AsyncToolRegistry implements AsyncToolTracker.
var _ AsyncToolTracker = (*AsyncToolRegistry)(nil)

// AsyncToolRegistry provides concurrent-safe tracking of async tool executions
type AsyncToolRegistry struct {
	// states stores *AsyncToolState keyed by callID
	states sync.Map
}

// NewAsyncToolRegistry creates a new AsyncToolRegistry
func NewAsyncToolRegistry() *AsyncToolRegistry {
	return &AsyncToolRegistry{}
}

// Register creates a new async tool state entry
func (r *AsyncToolRegistry) Register(sessionID, callID, toolName string, cancelFunc context.CancelFunc) {
	state := &AsyncToolState{
		CallID:     callID,
		ToolName:   toolName,
		Status:     StatusRunning,
		StartTime:  time.Now(),
		SessionID:  sessionID,
		CancelFunc: cancelFunc,
	}
	r.states.Store(callID, state)
}

// Unregister removes a tool state from the registry
func (r *AsyncToolRegistry) Unregister(callID string) {
	r.states.Delete(callID)
}

// Complete marks a tool execution as completed with the given result
func (r *AsyncToolRegistry) Complete(callID string, result any) {
	if val, ok := r.states.Load(callID); ok {
		state := val.(*AsyncToolState)
		state.Status = StatusCompleted
		state.EndTime = time.Now()
		state.Result = result
	}
}

// Fail marks a tool execution as failed with the given error message
func (r *AsyncToolRegistry) Fail(callID string, errMsg string) {
	if val, ok := r.states.Load(callID); ok {
		state := val.(*AsyncToolState)
		state.Status = StatusFailed
		state.EndTime = time.Now()
		state.Error = errMsg
	}
}

// GetStatus returns the current state of a tool execution
func (r *AsyncToolRegistry) GetStatus(callID string) (*AsyncToolState, error) {
	if val, ok := r.states.Load(callID); ok {
		return val.(*AsyncToolState), nil
	}
	return nil, errors.New("tool call not found: " + callID)
}

// Cancel cancels a running tool execution by callID
func (r *AsyncToolRegistry) Cancel(callID string) bool {
	if val, ok := r.states.Load(callID); ok {
		state := val.(*AsyncToolState)
		if state.Status == StatusRunning && state.CancelFunc != nil {
			state.CancelFunc()
			state.Status = StatusCancelled
			state.EndTime = time.Now()
			return true
		}
	}
	return false
}

// CancelSession cancels all running tools for a given sessionID
func (r *AsyncToolRegistry) CancelSession(sessionID string) int {
	cancelled := 0
	r.states.Range(func(key, value any) bool {
		state := value.(*AsyncToolState)
		if state.SessionID == sessionID && state.Status == StatusRunning {
			if state.CancelFunc != nil {
				state.CancelFunc()
				state.Status = StatusCancelled
				state.EndTime = time.Now()
				cancelled++
			}
		}
		return true
	})
	return cancelled
}

// ListBySession returns all tool states for a given sessionID
func (r *AsyncToolRegistry) ListBySession(sessionID string) []*AsyncToolState {
	var results []*AsyncToolState
	r.states.Range(func(key, value any) bool {
		state := value.(*AsyncToolState)
		if state.SessionID == sessionID {
			results = append(results, state)
		}
		return true
	})
	return results
}
