package skill

import (
	"testing"

	"github.com/copcon/core/capabilities/skill"
	"github.com/copcon/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSkills() []*skill.Skill {
	return []*skill.Skill{
		{
			Name:         "code-review",
			Description:  "Review code for quality and security issues",
			Instructions: "1. Read the code\n2. Check for bugs\n3. Report findings",
			ResourceFiles: []skill.ResourceFile{
				{Name: "scripts/check.sh", Path: "/skills/code-review/scripts/check.sh", Category: "scripts"},
				{Name: "references/style.md", Path: "/skills/code-review/references/style.md", Category: "references"},
			},
		},
		{
			Name:         "deploy",
			Description:  "Deploy application to production environment",
			Instructions: "1. Run tests\n2. Build artifact\n3. Deploy to cluster",
		},
	}
}

func TestSkillTool_Execute_List(t *testing.T) {
	st := NewSkillTool(newTestSkills())
	result, err := st.Execute(nil, map[string]any{"action": "list"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	data, ok := result.Data.(string)
	require.True(t, ok, "Data should be a string")
	assert.Contains(t, data, "Available Skills (2)")
	assert.Contains(t, data, "- code-review: Review code for quality and security issues")
	assert.Contains(t, data, "- deploy: Deploy application to production environment")
	assert.Contains(t, data, `Use skill(action="get", name="<name>") to get full instructions.`)
}

func TestSkillTool_Execute_ListEmpty(t *testing.T) {
	st := NewSkillTool([]*skill.Skill{})
	result, err := st.Execute(nil, map[string]any{"action": "list"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	data, ok := result.Data.(string)
	require.True(t, ok, "Data should be a string")
	assert.Equal(t, "No skills available.", data)
}

func TestSkillTool_Execute_Get(t *testing.T) {
	st := NewSkillTool(newTestSkills())
	result, err := st.Execute(nil, map[string]any{"action": "get", "name": "deploy"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	data, ok := result.Data.(string)
	require.True(t, ok, "Data should be a string")
	assert.Contains(t, data, "## Skill: deploy")
	assert.Contains(t, data, "**Description**: Deploy application to production environment")
	assert.Contains(t, data, "1. Run tests\n2. Build artifact\n3. Deploy to cluster")
	assert.NotContains(t, data, "### Resource Files")
}

func TestSkillTool_Execute_GetNotFound(t *testing.T) {
	st := NewSkillTool(newTestSkills())
	result, err := st.Execute(nil, map[string]any{"action": "get", "name": "nonexistent"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "skill not found: nonexistent", result.Error)
}

func TestSkillTool_Execute_GetWithResources(t *testing.T) {
	st := NewSkillTool(newTestSkills())
	result, err := st.Execute(nil, map[string]any{"action": "get", "name": "code-review"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	data, ok := result.Data.(string)
	require.True(t, ok, "Data should be a string")
	assert.Contains(t, data, "## Skill: code-review")
	assert.Contains(t, data, "**Description**: Review code for quality and security issues")
	assert.Contains(t, data, "1. Read the code")
	assert.Contains(t, data, "### Resource Files")
	assert.Contains(t, data, "- scripts/check.sh")
	assert.Contains(t, data, "- references/style.md")
}

func TestSkillTool_Execute_Search(t *testing.T) {
	st := NewSkillTool(newTestSkills())
	result, err := st.Execute(nil, map[string]any{"action": "search", "query": "code"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	data, ok := result.Data.(string)
	require.True(t, ok, "Data should be a string")
	assert.Contains(t, data, "- code-review: Review code for quality and security issues")
	assert.NotContains(t, data, "deploy")
}

func TestSkillTool_Execute_SearchNoMatch(t *testing.T) {
	st := NewSkillTool(newTestSkills())
	result, err := st.Execute(nil, map[string]any{"action": "search", "query": "nonexistent"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	data, ok := result.Data.(string)
	require.True(t, ok, "Data should be a string")
	assert.Equal(t, "No skills match query 'nonexistent'.", data)
}

func TestSkillTool_Execute_UnknownAction(t *testing.T) {
	st := NewSkillTool(newTestSkills())
	result, err := st.Execute(nil, map[string]any{"action": "delete"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "unknown action: delete", result.Error)
}

func TestSkillTool_Execute_MissingAction(t *testing.T) {
	st := NewSkillTool(newTestSkills())
	result, err := st.Execute(nil, map[string]any{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "action is required", result.Error)
}

func TestSkillTool_Metadata(t *testing.T) {
	st := NewSkillTool(nil)
	assert.Equal(t, "skill", st.Name())
}

// Compile-time interface check
var _ tool.Tool = (*SkillTool)(nil)
