package skill

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestSkill(t *testing.T, root, name, description string) {
	t.Helper()
	skillDir := filepath.Join(root, name)
	require.NoError(t, os.Mkdir(skillDir, 0o755))
	skillMD := "---\nname: " + name + "\ndescription: " + description + "\n---\nBody for " + name + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))
}

func createInvalidTestSkill(t *testing.T, root, name string) {
	t.Helper()
	skillDir := filepath.Join(root, name)
	require.NoError(t, os.Mkdir(skillDir, 0o755))
}

func TestDiscover_MultiplePaths_Dedup(t *testing.T) {
	highDir := t.TempDir()
	lowDir := t.TempDir()

	createTestSkill(t, highDir, "shared-skill", "high-priority desc")
	createTestSkill(t, lowDir, "shared-skill", "low-priority desc")
	createTestSkill(t, lowDir, "low-only-skill", "low only desc")

	d := NewDiscoverer("", nil, slog.Default())
	d.PrependPath(highDir)
	d.AddPath(lowDir)

	skills, err := d.Discover()
	require.NoError(t, err)
	require.Len(t, skills, 2)

	skillMap := make(map[string]string)
	for _, s := range skills {
		skillMap[s.Name] = s.Description
	}

	assert.Equal(t, "high-priority desc", skillMap["shared-skill"], "high priority path should win for duplicate name")
	assert.Equal(t, "low only desc", skillMap["low-only-skill"])
}

func TestDiscover_NonexistentPath(t *testing.T) {
	d := NewDiscoverer("", nil, slog.Default())
	d.AddPath("/nonexistent/path/xyz123")

	skills, err := d.Discover()
	assert.NoError(t, err)
	assert.Nil(t, skills)
}

func TestDiscover_EmptyPath(t *testing.T) {
	emptyDir := t.TempDir()

	d := NewDiscoverer("", nil, slog.Default())
	d.AddPath(emptyDir)

	skills, err := d.Discover()
	assert.NoError(t, err)
	assert.Nil(t, skills)
}

func TestDiscover_AddPath(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "added-skill", "dynamically added")

	d := NewDiscoverer("", nil, slog.Default())
	d.AddPath(dir)

	skills, err := d.Discover()
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "added-skill", skills[0].Name)
}

func TestDiscover_PrependPath(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()

	createTestSkill(t, firstDir, "dup-skill", "first priority")
	createTestSkill(t, secondDir, "dup-skill", "second priority")
	createTestSkill(t, secondDir, "unique-skill", "unique to second")

	d := NewDiscoverer("", nil, slog.Default())
	d.AddPath(secondDir)
	d.PrependPath(firstDir)

	skills, err := d.Discover()
	require.NoError(t, err)
	require.Len(t, skills, 2)

	skillMap := make(map[string]string)
	for _, s := range skills {
		skillMap[s.Name] = s.Description
	}

	assert.Equal(t, "first priority", skillMap["dup-skill"], "prepended path should have higher priority")
	assert.Equal(t, "unique to second", skillMap["unique-skill"])
}

func TestDiscover_InvalidSkill_Skipped(t *testing.T) {
	dir := t.TempDir()

	createTestSkill(t, dir, "valid-skill", "valid desc")
	createInvalidTestSkill(t, dir, "invalid-skill")

	d := NewDiscoverer("", nil, slog.Default())
	d.AddPath(dir)

	skills, err := d.Discover()
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "valid-skill", skills[0].Name)
}

func TestDiscover_ProjectRootExpansion(t *testing.T) {
	projectRoot := t.TempDir()

	skillsDir := filepath.Join(projectRoot, ".copcon", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	createTestSkill(t, skillsDir, "project-skill", "from project root")

	d := NewDiscoverer(projectRoot, nil, slog.Default())

	assert.NotEmpty(t, d.paths)

	skills, err := d.Discover()
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "project-skill", skills[0].Name)
}

func TestDiscover_ExtraPaths_HigherPriority(t *testing.T) {
	projectRoot := t.TempDir()

	defaultDir := filepath.Join(projectRoot, ".copcon", "skills")
	require.NoError(t, os.MkdirAll(defaultDir, 0o755))
	createTestSkill(t, defaultDir, "common-skill", "default priority")

	extraDir := t.TempDir()
	createTestSkill(t, extraDir, "common-skill", "extra priority")

	d := NewDiscoverer(projectRoot, []string{extraDir}, slog.Default())

	skills, err := d.Discover()
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "extra priority", skills[0].Description, "extra paths should have higher priority than defaults")
}
