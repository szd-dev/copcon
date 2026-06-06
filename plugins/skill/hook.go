package skill

import (
	"sort"
	"strings"

	"github.com/copcon/core/capabilities/skill"
	"github.com/copcon/core/hook"
)

// SkillInfoHook appends a summary of available skills to the system prompt
// when the OnSystemPrompt hook point fires.
type SkillInfoHook struct {
	skills []*skill.Skill
}

// NewSkillInfoHook creates a hook that injects skill metadata into the system prompt.
func NewSkillInfoHook(skills []*skill.Skill) *SkillInfoHook {
	return &SkillInfoHook{skills: skills}
}

func (h *SkillInfoHook) Name() string         { return "skill_info" }
func (h *SkillInfoHook) Points() []hook.HookPoint { return []hook.HookPoint{hook.OnSystemPrompt} }
func (h *SkillInfoHook) Priority() int         { return 60 }

func (h *SkillInfoHook) Execute(ctx *hook.HookContext) error {
	if ctx.SystemPrompt == nil {
		return nil
	}

	if len(h.skills) == 0 {
		return nil
	}

	sorted := make([]*skill.Skill, len(h.skills))
	copy(sorted, h.skills)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var b strings.Builder
	b.WriteString("\n\n## Available Skills\n\n")
	b.WriteString("The following skills provide specialized guidance. Use the `skill` tool to activate them.\n\n")

	for _, s := range sorted {
		b.WriteString("- **")
		b.WriteString(s.Name)
		b.WriteString("**: ")
		b.WriteString(s.Description)
		b.WriteString("\n")
	}

	*ctx.SystemPrompt = *ctx.SystemPrompt + b.String()

	return nil
}
