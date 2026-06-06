package skill

import (
	"fmt"

	"github.com/copcon/core/capabilities"
	skilltypes "github.com/copcon/core/capabilities/skill"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

// Config is the configuration for the Skill plugin.
type Config struct {
	ProjectRoot string   // project root directory
	ExtraPaths  []string // additional search paths (high priority, before defaults)
}

// SkillModule implements ModuleCapability.
type SkillModule struct {
	cfg        Config
	discoverer *Discoverer
	skills     []*skilltypes.Skill
}

var _ capabilities.ModuleCapability = (*SkillModule)(nil)

// NewSkillModule creates a new SkillModule with the given config.
func NewSkillModule(cfg Config) *SkillModule {
	return &SkillModule{cfg: cfg}
}

func (m *SkillModule) Name() string                        { return "modules.skills" }
func (m *SkillModule) Type() capabilities.CapabilityType   { return capabilities.CapabilityTypeModule }
func (m *SkillModule) DependsOn() []string                 { return nil }

func (m *SkillModule) NewHooks(deps capabilities.CapabilityDeps) ([]hook.Hook, error) {
	m.discoverer = NewDiscoverer(m.cfg.ProjectRoot, m.cfg.ExtraPaths, deps.Logger)
	skills, err := m.discoverer.Discover()
	if err != nil {
		return nil, fmt.Errorf("discover skills: %w", err)
	}
	m.skills = skills
	return []hook.Hook{NewSkillInfoHook(skills)}, nil
}

func (m *SkillModule) NewTools(deps capabilities.CapabilityDeps) ([]tool.Tool, error) {
	return []tool.Tool{NewSkillTool(m.skills)}, nil
}
