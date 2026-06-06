package skill

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()

	skill1Dir := filepath.Join(tmpDir, "code-review")
	require.NoError(t, os.MkdirAll(skill1Dir, 0755))
	writeSKILLmd(t, skill1Dir, "code-review", "Review code for style, bugs, and security issues.",
		"## Code Review Instructions\n\n1. Check type safety\n2. Check error handling\n3. Check security\n")

	skill2Dir := filepath.Join(tmpDir, "deploy")
	require.NoError(t, os.MkdirAll(skill2Dir, 0755))
	writeSKILLmd(t, skill2Dir, "deploy", "Deploy application to staging or production environments.",
		"## Deploy Instructions\n\n1. Verify tests pass\n2. Build artifacts\n3. Deploy\n")

	t.Run("Discovery", func(t *testing.T) {
		discoverer := NewDiscoverer("", []string{tmpDir}, slog.Default())
		skills, err := discoverer.Discover()
		require.NoError(t, err)
		assert.Len(t, skills, 2, "expected exactly 2 skills from discovery")

		skillNames := make(map[string]bool)
		for _, s := range skills {
			skillNames[s.Name] = true
		}
		assert.True(t, skillNames["code-review"])
		assert.True(t, skillNames["deploy"])
	})

	t.Run("Registry", func(t *testing.T) {
		reg := capabilities.NewRegistry()
		RegisterCapabilities(reg, Config{
			ExtraPaths: []string{tmpDir},
		})
		cap, ok := reg.Get("modules.skills")
		require.True(t, ok, "modules.skills should be registered")
		assert.Equal(t, capabilities.CapabilityTypeModule, cap.Type(), "must be CapabilityTypeModule")
	})

	t.Run("Hook_SystemPrompt", func(t *testing.T) {
		d := NewDiscoverer("", []string{tmpDir}, slog.Default())
		skills, err := d.Discover()
		require.NoError(t, err)
		h := NewSkillInfoHook(skills)

		prompt := "You are a helpful assistant."
		ctx := &hook.HookContext{SystemPrompt: &prompt}
		hookErr := h.Execute(ctx)
		require.NoError(t, hookErr)

		assert.Contains(t, prompt, "## Available Skills", "system prompt must contain skill section")
		assert.Contains(t, prompt, "code-review", "must list code-review skill")
		assert.Contains(t, prompt, "deploy", "must list deploy skill")
		assert.NotContains(t, prompt, "## Code Review Instructions", "hook should NOT inject instructions")
	})

	t.Run("Tool_Operations", func(t *testing.T) {
		d := NewDiscoverer("", []string{tmpDir}, slog.Default())
		skills, err := d.Discover()
		require.NoError(t, err)
		tool := NewSkillTool(skills)

		listResult, err := tool.Execute(nil, map[string]any{"action": "list"})
		require.NoError(t, err)
		assert.True(t, listResult.Success)
		assert.Contains(t, listResult.Data, "code-review")
		assert.Contains(t, listResult.Data, "deploy")

		getResult, err := tool.Execute(nil, map[string]any{"action": "get", "name": "code-review"})
		require.NoError(t, err)
		assert.True(t, getResult.Success)
		assert.Contains(t, getResult.Data, "## Code Review Instructions")

		searchResult, err := tool.Execute(nil, map[string]any{"action": "search", "query": "deploy"})
		require.NoError(t, err)
		assert.True(t, searchResult.Success)
		assert.Contains(t, searchResult.Data, "deploy")
		assert.NotContains(t, searchResult.Data, "code-review")
	})
}

func writeSKILLmd(t *testing.T, dir, name, description, body string) {
	t.Helper()
	content := fmt.Sprintf(`---
name: %s
description: %s
---

%s`, name, description, body)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644))
}