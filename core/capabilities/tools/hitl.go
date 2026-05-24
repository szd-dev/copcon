package tools

import (
	"fmt"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

type ConfirmActionTool struct{}

func NewConfirmActionTool() *ConfirmActionTool {
	return &ConfirmActionTool{}
}

func (t *ConfirmActionTool) Name() string { return "confirm_action" }

func (t *ConfirmActionTool) Description() string {
	return "Ask the user to confirm or decline a proposed action. Use this before executing potentially dangerous or irreversible operations."
}

func (t *ConfirmActionTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The message to display to the user asking for confirmation",
			},
			"summary": map[string]any{
				"type":        "string",
				"description": "A brief summary of the action being confirmed",
			},
		},
		"required": []string{"message"},
	}
}

func (t *ConfirmActionTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	message, _ := args["message"].(string)
	summary, _ := args["summary"].(string)

	if message == "" {
		return nil, fmt.Errorf("message is required")
	}

	resp, err := chatCtx.RequestInput(iface.InputRequest{
		Type:     iface.InterruptApproval,
		Message:  message,
		Summary:  summary,
		ToolName: t.Name(),
		ToolArgs: args,
	})
	if err != nil {
		return nil, fmt.Errorf("confirmation request failed: %w", err)
	}

	if resp.Action != "approve" {
		return &tool.ToolResult{Success: false, Error: "user declined the action"}, nil
	}

	return &tool.ToolResult{Success: true, Data: "action approved"}, nil
}

type AskUserTool struct{}

func NewAskUserTool() *AskUserTool {
	return &AskUserTool{}
}

func (t *AskUserTool) Name() string { return "ask_user" }

func (t *AskUserTool) Description() string {
	return "Ask the user a question and wait for their response. Use this when you need additional information or clarification from the user to proceed."
}

func (t *AskUserTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The question to ask the user",
			},
			"options": map[string]any{
				"type": "array",
				"description": "List of options for the user to choose from. Each option has a label (displayed to user) and value (returned as the answer). Provide this when you want the user to select from predefined choices rather than type free text.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"label": map[string]any{
							"type":        "string",
							"description": "Display text shown to the user for this option",
						},
						"value": map[string]any{
							"type":        "string",
							"description": "The value returned when the user selects this option",
						},
					},
					"required": []string{"label", "value"},
				},
			},
		},
		"required": []string{"message"},
	}
}

func (t *AskUserTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	message, _ := args["message"].(string)
	optionsRaw, _ := args["options"].([]any)

	if message == "" {
		return nil, fmt.Errorf("message is required")
	}

	var inputSchema map[string]any
	if len(optionsRaw) > 0 {
		enumValues := make([]string, 0, len(optionsRaw))
		optionLabels := make(map[string]string)
		for _, opt := range optionsRaw {
			if m, ok := opt.(map[string]any); ok {
				label, _ := m["label"].(string)
				value, _ := m["value"].(string)
				if value != "" {
					enumValues = append(enumValues, value)
					if label != "" {
						optionLabels[value] = label
					}
				}
			}
		}
		if len(enumValues) > 0 {
			inputSchema = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"answer": map[string]any{
						"type":         "string",
						"description":  "Your choice",
						"enum":         enumValues,
						"optionLabels": optionLabels,
					},
				},
				"required": []string{"answer"},
			}
		}
	}

	resp, err := chatCtx.RequestInput(iface.InputRequest{
		Type:        iface.InterruptQuestion,
		Message:     message,
		InputSchema: inputSchema,
		ToolName:    t.Name(),
		ToolArgs:    args,
	})
	if err != nil {
		return nil, fmt.Errorf("question request failed: %w", err)
	}

	if resp.Action == "cancel" {
		return &tool.ToolResult{Success: false, Error: "user cancelled the input request"}, nil
	}

	return &tool.ToolResult{Success: true, Data: resp.Content}, nil
}

func init() {
	capabilities.Register(&confirmActionCapability{})
	capabilities.Register(&askUserCapability{})
}

type confirmActionCapability struct{}

func (c *confirmActionCapability) Name() string                      { return "tools.confirm_action" }
func (c *confirmActionCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *confirmActionCapability) DependsOn() []string               { return nil }
func (c *confirmActionCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return NewConfirmActionTool(), nil
}

type askUserCapability struct{}

func (c *askUserCapability) Name() string                      { return "tools.ask_user" }
func (c *askUserCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeTool }
func (c *askUserCapability) DependsOn() []string               { return nil }
func (c *askUserCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return NewAskUserTool(), nil
}