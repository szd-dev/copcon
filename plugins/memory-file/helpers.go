package memoryfile

import (
	"encoding/json"
	"fmt"

	"github.com/copcon/core/tool"
)

func successResult(data map[string]any) (*tool.ToolResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("failed to marshal response: %v", err)}, nil
	}

	return &tool.ToolResult{
		Success: true,
		Data: map[string]any{
			"response": string(jsonData),
		},
	}, nil
}

func errorResult(message string) (*tool.ToolResult, error) {
	return &tool.ToolResult{Success: false, Error: message}, nil
}
