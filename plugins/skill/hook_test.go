package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/capabilities/skill"
	"github.com/copcon/core/hook"
)

func TestSkillInfoHook_Execute_WithSkills(t *testing.T) {
	skills := []*skill.Skill{
		{Name: "code-review", Description: "Reviews code for quality and security issues"},
		{Name: "testing", Description: "Generates comprehensive test suites"},
	}
	h := NewSkillInfoHook(skills)

	prompt := "You are an assistant."
	ctx := &hook.HookContext{
		SystemPrompt: &prompt,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, *ctx.SystemPrompt, "## Available Skills")
	assert.Contains(t, *ctx.SystemPrompt, "code-review")
	assert.Contains(t, *ctx.SystemPrompt, "Reviews code for quality and security issues")
	assert.Contains(t, *ctx.SystemPrompt, "testing")
	assert.Contains(t, *ctx.SystemPrompt, "Generates comprehensive test suites")
	assert.Contains(t, *ctx.SystemPrompt, "`skill` tool")
}

func TestSkillInfoHook_Execute_NoSkills(t *testing.T) {
	h := NewSkillInfoHook([]*skill.Skill{})

	prompt := "You are an assistant."
	ctx := &hook.HookContext{
		SystemPrompt: &prompt,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "You are an assistant.", *ctx.SystemPrompt)
}

func TestSkillInfoHook_Execute_NilPrompt(t *testing.T) {
	h := NewSkillInfoHook([]*skill.Skill{
		{Name: "test-skill", Description: "A test skill"},
	})

	ctx := &hook.HookContext{
		SystemPrompt: nil,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)
}

func TestSkillInfoHook_Execute_Alphabetical(t *testing.T) {
	skills := []*skill.Skill{
		{Name: "zulu", Description: "Z skill"},
		{Name: "alpha", Description: "A skill"},
		{Name: "middle", Description: "M skill"},
	}
	h := NewSkillInfoHook(skills)

	prompt := "You are an assistant."
	ctx := &hook.HookContext{
		SystemPrompt: &prompt,
	}

	err := h.Execute(ctx)
	require.NoError(t, err)

	result := *ctx.SystemPrompt
	idxAlpha := strings.Index(result, "alpha")
	idxMiddle := strings.Index(result, "middle")
	idxZulu := strings.Index(result, "zulu")

	assert.Greater(t, idxMiddle, idxAlpha, "middle should appear after alpha")
	assert.Greater(t, idxZulu, idxMiddle, "zulu should appear after middle")
}

func TestSkillInfoHook_Metadata(t *testing.T) {
	h := NewSkillInfoHook(nil)
	assert.Equal(t, "skill_info", h.Name())
	assert.Equal(t, []hook.HookPoint{hook.OnSystemPrompt}, h.Points())
	assert.Equal(t, 60, h.Priority())
}
