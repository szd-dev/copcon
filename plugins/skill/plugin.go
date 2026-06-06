package skill

import (
	"fmt"
	"log/slog"

	skilltypes "github.com/copcon/core/capabilities/skill"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/tool"
)

type toolNameWrapper struct {
	tool.Tool
	newName string
}

func (w *toolNameWrapper) Name() string { return w.newName }

type hookNameWrapper struct {
	hook.Hook
	newName string
}

func (w *hookNameWrapper) Name() string { return w.newName }

// Config is the configuration for the Skill plugin.
type Config struct {
	ProjectRoot string   // project root directory
	ExtraPaths  []string // additional search paths (high priority, before defaults)
}

// SkillPlugin implements plugin.Plugin for the skill capability.
type SkillPlugin struct {
	cfg        Config
	discoverer *Discoverer
	skills     []*skilltypes.Skill
	logger     *slog.Logger
}

var _ plugin.Plugin = (*SkillPlugin)(nil)

// NewPlugin creates a new SkillPlugin with the given config.
func NewPlugin(cfg Config) plugin.Plugin {
	return &SkillPlugin{cfg: cfg}
}

func (p *SkillPlugin) Name() string { return "skill" }

func (p *SkillPlugin) Tools() []tool.Tool {
	return []tool.Tool{
		&toolNameWrapper{Tool: NewSkillTool(p.skills), newName: "skill.tool.skill"},
	}
}

func (p *SkillPlugin) Hooks() []hook.Hook {
	return []hook.Hook{
		&hookNameWrapper{Hook: NewSkillInfoHook(p.skills), newName: "skill.hook.skill_info"},
	}
}

func (p *SkillPlugin) Init(deps plugin.PluginDeps) error {
	p.logger = deps.Logger
	if p.logger == nil {
		p.logger = slog.Default()
	}

	p.discoverer = NewDiscoverer(p.cfg.ProjectRoot, p.cfg.ExtraPaths, p.logger)
	skills, err := p.discoverer.Discover()
	if err != nil {
		return fmt.Errorf("discover skills: %w", err)
	}
	p.skills = skills
	return nil
}

// GetConfig returns the plugin configuration.
func (p *SkillPlugin) GetConfig() Config {
	return p.cfg
}
