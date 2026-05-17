package entity

import "time"

// UIPartType 定义UI消息部分的类型
type UIPartType string

const (
	// UIPartText 文本内容部分
	UIPartText UIPartType = "text"
	// UIPartReasoning 推理/思考过程部分
	UIPartReasoning UIPartType = "reasoning"
	// UIPartToolCall 工具调用部分
	UIPartToolCall UIPartType = "tool-call"
	// UIPartStepStart 步骤开始标记部分
	UIPartStepStart UIPartType = "step-start"
)

// UIPartState 定义UI消息部分的状态
type UIPartState string

const (
	// UIPartStateStreaming 正在流式传输中
	UIPartStateStreaming UIPartState = "streaming"
	// UIPartStateDone 传输完成
	UIPartStateDone UIPartState = "done"
	// UIPartStatePending 等待开始
	UIPartStatePending UIPartState = "pending"
	// UIPartStateRunning 正在执行中
	UIPartStateRunning UIPartState = "running"
	// UIPartStateComplete 执行完成
	UIPartStateComplete UIPartState = "complete"
	// UIPartStateError 执行出错
	UIPartStateError UIPartState = "error"
)

// UIMessage 定义UI层消息结构，使用parts数组支持富内容展示
type UIMessage struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	// Role 消息角色，仅支持 "user" 和 "assistant"
	Role     string     `json:"role"`
	Steps    []UIStep   `json:"steps"`
	Parts    []UIPart   `json:"parts"` // Deprecated: use Steps instead; kept for transition period
	Metadata UIMetadata `json:"metadata"`
}

// UIStep 定义UI消息中的步骤，每个步骤包含一组Parts
type UIStep struct {
	Parts []UIPart    `json:"parts"`
	State UIPartState `json:"state"`
}

// UIPart 定义UI消息中的内容部分，不同类型使用不同字段组合
type UIPart struct {
	Type       UIPartType  `json:"type"`
	StepIndex  int         `json:"stepIndex,omitempty"`
	Text       string      `json:"text,omitempty"`
	State      UIPartState `json:"state,omitempty"`
	ToolCallID string      `json:"toolCallId,omitempty"`
	ToolName   string      `json:"toolName,omitempty"`
	Args       string      `json:"args,omitempty"`
	Output     string      `json:"output,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// UIMetadata 定义UI消息的元数据
type UIMetadata struct {
	CreatedAt  time.Time `json:"createdAt"`
	Model      string    `json:"model,omitempty"`
	TokenCount int       `json:"tokenCount,omitempty"`
	DurationMs int64     `json:"durationMs,omitempty"`
}
