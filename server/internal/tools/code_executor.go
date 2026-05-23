package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

const (
	CodeExecutionTimeout = 30 * time.Second
)

type CodeExecutor struct {
	timeout time.Duration
}

func NewCodeExecutor() *CodeExecutor {
	return &CodeExecutor{
		timeout: CodeExecutionTimeout,
	}
}

func (t *CodeExecutor) Name() string {
	return "code_executor"
}

func (t *CodeExecutor) Description() string {
	return "Execute Python or JavaScript code in a sandboxed environment"
}

func (t *CodeExecutor) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language": map[string]any{
				"type":        "string",
				"enum":        []string{"python", "javascript"},
				"description": "Programming language",
			},
			"code": map[string]any{
				"type":        "string",
				"description": "Code to execute",
			},
		},
		"required": []string{"language", "code"},
	}
}

func (t *CodeExecutor) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	language, ok := args["language"].(string)
	if !ok {
		return &tool.ToolResult{Success: false, Error: "language is required"}, nil
	}

	code, ok := args["code"].(string)
	if !ok {
		return &tool.ToolResult{Success: false, Error: "code is required"}, nil
	}

	execCtx, cancel := context.WithTimeout(chatCtx.Context(), t.timeout)
	defer cancel()

	var cmd *exec.Cmd

	switch language {
	case "python":
		cmd = exec.CommandContext(execCtx, "python3", "-c", code)
	case "javascript":
		cmd = exec.CommandContext(execCtx, "node", "-e", code)
	default:
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("unsupported language: %s", language)}, nil
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &tool.ToolResult{
			Success: false,
			Data: map[string]any{
				"stdout":    "",
				"stderr":    string(output),
				"exit_code": 1,
				"error":     err.Error(),
			},
		}, nil
	}

	return &tool.ToolResult{
		Success: true,
		Data: map[string]any{
			"stdout":    string(output),
			"stderr":    "",
			"exit_code": 0,
		},
	}, nil
}

var allowedShellCommands = map[string]bool{
	"ls":     true,
	"cat":    true,
	"echo":   true,
	"pwd":    true,
	"date":   true,
	"whoami": true,
	"which":  true,
	"head":   true,
	"tail":   true,
	"wc":     true,
	"find":   true,
	"grep":   true,
}

type ShellExecutor struct {
	timeout time.Duration
}

func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{
		timeout: 10 * time.Second,
	}
}

func (t *ShellExecutor) Name() string {
	return "shell_executor"
}

func (t *ShellExecutor) Description() string {
	return "Execute allowed shell commands (whitelist enforced)"
}

func (t *ShellExecutor) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ShellExecutor) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	command, ok := args["command"].(string)
	if !ok {
		return &tool.ToolResult{Success: false, Error: "command is required"}, nil
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return &tool.ToolResult{Success: false, Error: "empty command"}, nil
	}

	cmdName := parts[0]
	if !allowedShellCommands[cmdName] {
		return &tool.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("command '%s' is not in the allowed list", cmdName),
		}, nil
	}

	execCtx, cancel := context.WithTimeout(chatCtx.Context(), t.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return &tool.ToolResult{
			Success: false,
			Data: map[string]any{
				"output":    string(output),
				"exit_code": 1,
				"error":     err.Error(),
			},
		}, nil
	}

	return &tool.ToolResult{
		Success: true,
		Data: map[string]any{
			"output":    string(output),
			"exit_code": 0,
		},
	}, nil
}
