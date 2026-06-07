package skill

import (
	"log/slog"
	"os"
	"testing"

	"github.com/copcon/core/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPlugin(t *testing.T) {
	cfg := Config{ProjectRoot: "/tmp"}
	p := NewPlugin(cfg)

	require.NotNil(t, p)
	assert.Equal(t, "skill", p.Name())
}

func TestSkillPlugin_Name(t *testing.T) {
	p := NewPlugin(Config{})
	assert.Equal(t, "skill", p.Name())
}

func TestSkillPlugin_Tools_BeforeInit(t *testing.T) {
	p := NewPlugin(Config{})
	tools := p.Tools()
	assert.Len(t, tools, 1)
	assert.Equal(t, "skill.tool.skill", tools[0].Name())
}

func TestSkillPlugin_Hooks_BeforeInit(t *testing.T) {
	p := NewPlugin(Config{})
	hooks := p.Hooks()
	assert.Len(t, hooks, 1)
	assert.Equal(t, "skill.hook.skill_info", hooks[0].Name())
}

func TestSkillPlugin_Init_InjectsLogger(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{ProjectRoot: tmpDir}
	p := NewPlugin(cfg)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	deps := plugin.PluginDeps{Logger: logger}

	err := p.Init(deps)
	require.NoError(t, err)

	sp := p.(*SkillPlugin)
	assert.Equal(t, logger, sp.logger)
}

func TestSkillPlugin_Init_DefaultLogger(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{ProjectRoot: tmpDir}
	p := NewPlugin(cfg)

	err := p.Init(plugin.PluginDeps{})
	require.NoError(t, err)

	sp := p.(*SkillPlugin)
	assert.NotNil(t, sp.logger)
}

func TestSkillPlugin_Tools_AfterInit(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{ProjectRoot: tmpDir}
	p := NewPlugin(cfg)

	err := p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	tools := p.Tools()
	assert.Len(t, tools, 1)
	assert.Equal(t, "skill.tool.skill", tools[0].Name())
}

func TestSkillPlugin_Hooks_AfterInit(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{ProjectRoot: tmpDir}
	p := NewPlugin(cfg)

	err := p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	hooks := p.Hooks()
	assert.Len(t, hooks, 1)
	assert.Equal(t, "skill.hook.skill_info", hooks[0].Name())
}

func TestSkillPlugin_Tools_PreservesDelegate(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{ProjectRoot: tmpDir}
	p := NewPlugin(cfg)

	err := p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	tools := p.Tools()
	require.Len(t, tools, 1)

	assert.Equal(t, "skill.tool.skill", tools[0].Name())
	assert.NotEmpty(t, tools[0].Description())
	assert.NotNil(t, tools[0].InputSchema())
}

func TestSkillPlugin_Hooks_PreservesDelegate(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{ProjectRoot: tmpDir}
	p := NewPlugin(cfg)

	err := p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	hooks := p.Hooks()
	require.Len(t, hooks, 1)

	assert.Equal(t, "skill.hook.skill_info", hooks[0].Name())
	assert.NotNil(t, hooks[0].Points())
	assert.NotEmpty(t, hooks[0].Points())
}

func TestSkillPlugin_GetConfig(t *testing.T) {
	cfg := Config{
		ProjectRoot: "/some/path",
		ExtraPaths:  []string{"/extra1", "/extra2"},
	}
	p := NewPlugin(cfg)

	got := p.(*SkillPlugin).GetConfig()
	assert.Equal(t, cfg.ProjectRoot, got.ProjectRoot)
	assert.Equal(t, cfg.ExtraPaths, got.ExtraPaths)
}
