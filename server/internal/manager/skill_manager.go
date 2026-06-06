package manager

import (
	"fmt"

	skilltypes "github.com/copcon/core/capabilities/skill"
	"github.com/copcon/plugins/skill"
	"github.com/copcon/server/internal/api"
)

type SkillManager struct {
	plugin *skill.SkillPlugin
}

var _ api.SkillProvider = (*SkillManager)(nil)

func NewSkillManager(p *skill.SkillPlugin) *SkillManager {
	return &SkillManager{plugin: p}
}

func (m *SkillManager) ListSkills() ([]api.SkillInfo, error) {
	skills := m.plugin.Skills()
	result := make([]api.SkillInfo, len(skills))
	for i, s := range skills {
		result[i] = api.SkillInfo{
			Name:         s.Name,
			Description:  s.Description,
			Enabled:      m.plugin.IsSkillEnabled(s.Name),
			Source:       s.Source,
			Metadata:     s.Metadata,
			AllowedTools: s.AllowedTools,
		}
	}
	return result, nil
}

func (m *SkillManager) GetSkill(name string) (*api.SkillDetail, error) {
	s, ok := m.findSkill(name)
	if !ok {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return &api.SkillDetail{
		SkillInfo: api.SkillInfo{
			Name:         s.Name,
			Description:  s.Description,
			Enabled:      m.plugin.IsSkillEnabled(s.Name),
			Source:       s.Source,
			Metadata:     s.Metadata,
			AllowedTools: s.AllowedTools,
		},
		Instructions:  s.Instructions,
		ResourceFiles: toResourceFileInfos(s.ResourceFiles),
	}, nil
}

func (m *SkillManager) SetSkillEnabled(name string, enabled bool) error {
	if _, ok := m.findSkill(name); !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	m.plugin.SetSkillEnabled(name, enabled)
	return nil
}

func (m *SkillManager) findSkill(name string) (*skilltypes.Skill, bool) {
	for _, s := range m.plugin.Skills() {
		if s.Name == name {
			return s, true
		}
	}
	return nil, false
}

func toResourceFileInfos(files []skilltypes.ResourceFile) []api.ResourceFileInfo {
	result := make([]api.ResourceFileInfo, len(files))
	for i, rf := range files {
		result[i] = api.ResourceFileInfo{
			Name:     rf.Name,
			Path:     rf.Path,
			Category: rf.Category,
		}
	}
	return result
}
