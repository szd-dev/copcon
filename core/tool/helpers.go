package tool

import (
	"encoding/json"
	"fmt"
)

// SuccessResult creates a successful ToolResult with the given data marshalled as JSON.
func SuccessResult(data map[string]any) (*ToolResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to marshal response: %v", err)}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"response": string(jsonData),
		},
	}, nil
}

// ErrorResult creates a failed ToolResult with the given error message.
func ErrorResult(message string) (*ToolResult, error) {
	return &ToolResult{Success: false, Error: message}, nil
}