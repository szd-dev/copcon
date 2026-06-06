package skill

import (
	"os"
	"path/filepath"
	"testing"

	skilltypes "github.com/copcon/core/capabilities/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"
)

func TestParseSkill_ValidSKILLmd(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.Mkdir(skillDir, 0o755))

	skillMD := `---
name: my-skill
description: A useful skill for doing things
license: MIT
metadata:
  author: copcon
  version: "1.0"
allowed-tools: shell_executor code_executor
---
This is the instruction body.
It can have multiple lines.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))

	got, err := ParseSkill(skillDir)
	require.NoError(t, err)

	assert.Equal(t, "my-skill", got.Name)
	assert.Equal(t, "A useful skill for doing things", got.Description)
	assert.Equal(t, "MIT", got.License)
	assert.Equal(t, "shell_executor code_executor", got.AllowedTools)
	assert.Equal(t, "This is the instruction body.\nIt can have multiple lines.", got.Instructions)
	assert.Equal(t, skillDir, got.DirPath)
	assert.Equal(t, "copcon", got.Metadata["author"])
	assert.Equal(t, "1.0", got.Metadata["version"])
}

func TestParseSkill_MissingSKILLmd(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.Mkdir(skillDir, 0o755))

	_, err := ParseSkill(skillDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SKILL.md")
}

func TestParseSkill_NameMismatch(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.Mkdir(skillDir, 0o755))

	skillMD := `---
name: other-name
description: Some description
---
Body
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))

	_, err := ParseSkill(skillDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestParseSkill_EmptyDescription(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.Mkdir(skillDir, 0o755))

	skillMD := `---
name: my-skill
description: ""
---
Body
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))

	_, err := ParseSkill(skillDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description")
}

func TestParseSkill_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.Mkdir(skillDir, 0o755))

	skillMD := `---
name: my-skill
description: desc
  bad indent: [unclosed
---
Body
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))

	_, err := ParseSkill(skillDir)
	require.Error(t, err)
}

func TestParseSkill_WithResourceFiles(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.Mkdir(skillDir, 0o755))

	skillMD := `---
name: my-skill
description: A skill with resources
---
Body
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))

	require.NoError(t, os.Mkdir(filepath.Join(skillDir, "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("#!/bin/bash"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(skillDir, "references"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "references", "doc.md"), []byte("# Doc"), 0o644))

	got, err := ParseSkill(skillDir)
	require.NoError(t, err)

	require.Len(t, got.ResourceFiles, 2)

	rfMap := make(map[string]skilltypes.ResourceFile)
	for _, rf := range got.ResourceFiles {
		rfMap[rf.Name] = rf
	}

	assert.Contains(t, rfMap, "scripts/run.sh")
	assert.Equal(t, "scripts", rfMap["scripts/run.sh"].Category)
	assert.Equal(t, filepath.Join(skillDir, "scripts", "run.sh"), rfMap["scripts/run.sh"].Path)

	assert.Contains(t, rfMap, "references/doc.md")
	assert.Equal(t, "references", rfMap["references/doc.md"].Category)
	assert.Equal(t, filepath.Join(skillDir, "references", "doc.md"), rfMap["references/doc.md"].Path)
}

func TestParseSkillDir_MultipleSkills(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"skill-a", "skill-b"} {
		skillDir := filepath.Join(root, name)
		require.NoError(t, os.Mkdir(skillDir, 0o755))
		skillMD := "---\nname: " + name + "\ndescription: " + name + " desc\n---\nBody\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))
	}

	skills, err := ParseSkillDir(root, slog.Default())
	require.NoError(t, err)
	require.Len(t, skills, 2)

	names := map[string]bool{skills[0].Name: true, skills[1].Name: true}
	assert.True(t, names["skill-a"])
	assert.True(t, names["skill-b"])
}

func TestParseSkillDir_SkipsInvalid(t *testing.T) {
	root := t.TempDir()

	validDir := filepath.Join(root, "valid-skill")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validMD := "---\nname: valid-skill\ndescription: valid desc\n---\nBody\n"
	require.NoError(t, os.WriteFile(filepath.Join(validDir, "SKILL.md"), []byte(validMD), 0o644))

	invalidDir := filepath.Join(root, "invalid-skill")
	require.NoError(t, os.Mkdir(invalidDir, 0o755))

	skills, err := ParseSkillDir(root, slog.Default())
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "valid-skill", skills[0].Name)
}

func TestParseSkillDir_NonexistentPath(t *testing.T) {
	skills, err := ParseSkillDir("/nonexistent/path/xyz", slog.Default())
	assert.Nil(t, err)
	assert.Nil(t, skills)
}
