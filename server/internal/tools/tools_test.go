package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/copcon/server/internal/testutil"
)

func TestCodeExecutor_Python(t *testing.T) {
	executor := NewCodeExecutor()

	chatCtx := testutil.NewMockChatContext(context.Background(), "", "")
	result, err := executor.Execute(chatCtx, map[string]any{
		"language": "python",
		"code":     "print(2 + 2)",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Data.(map[string]any)["stdout"], "4")
}

func TestCodeExecutor_JavaScript(t *testing.T) {
	executor := NewCodeExecutor()

	chatCtx := testutil.NewMockChatContext(context.Background(), "", "")
	result, err := executor.Execute(chatCtx, map[string]any{
		"language": "javascript",
		"code":     "console.log(3 * 3)",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
}

func TestCodeExecutor_UnsupportedLanguage(t *testing.T) {
	executor := NewCodeExecutor()

	chatCtx := testutil.NewMockChatContext(context.Background(), "", "")
	result, err := executor.Execute(chatCtx, map[string]any{
		"language": "ruby",
		"code":     "puts 'hello'",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
}

func TestShellExecutor_AllowedCommand(t *testing.T) {
	executor := NewShellExecutor()

	chatCtx := testutil.NewMockChatContext(context.Background(), "", "")
	result, err := executor.Execute(chatCtx, map[string]any{
		"command": "echo hello",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Data.(map[string]any)["output"], "hello")
}

func TestShellExecutor_ForbiddenCommand(t *testing.T) {
	executor := NewShellExecutor()

	chatCtx := testutil.NewMockChatContext(context.Background(), "", "")
	result, err := executor.Execute(chatCtx, map[string]any{
		"command": "rm -rf /",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
}

func TestFileOps_ListDir(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewFileOps(tmpDir)

	chatCtx := testutil.NewMockChatContext(context.Background(), "", "")
	result, err := executor.Execute(chatCtx, map[string]any{
		"operation": "list",
		"path":      tmpDir,
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
}
