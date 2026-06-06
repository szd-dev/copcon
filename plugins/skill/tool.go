package skill

import (
	"fmt"
	"sort"
	"strings"

	"github.com/copcon/core/capabilities/skill"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

type SkillTool struct {
	skills map[string]*skill.Skill
}

func NewSkillTool(skills []*skill.Skill) *SkillTool {
	m := make(map[string]*skill.Skill)
	for _, s := range skills {
		m[s.Name] = s
	}
	return &SkillTool{skills: m}
}

func (t *SkillTool) Name() string { return "skill" }

func (t *SkillTool) Description() string {
	return "Get information about available skills. Use action='list' to see all skills, action='get' to get full instructions for a specific skill, or action='search' to find skills by keyword."
}

func (t *SkillTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "get", "search"},
				"description": "list: all skills, get: full instructions by name, search: by keyword",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name (required for action='get')",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search keyword (required for action='search')",
			},
		},
		"required": []string{"action"},
	}
}

func (t *SkillTool) Execute(_ iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return &tool.ToolResult{Success: false, Error: "action is required"}, nil
	}

	switch action {
	case "list":
		return t.executeList(), nil
	case "get":
		return t.executeGet(args), nil
	case "search":
		return t.executeSearch(args), nil
	default:
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("unknown action: %s", action)}, nil
	}
}

func (t *SkillTool) executeList() *tool.ToolResult {
	if len(t.skills) == 0 {
		return &tool.ToolResult{Success: true, Data: "No skills available."}
	}

	names := make([]string, 0, len(t.skills))
	for name := range t.skills {
		names = append(names, name)
	}
	sort.Strings(names)

	var lines []string
	for _, name := range names {
		s := t.skills[name]
		lines = append(lines, fmt.Sprintf("- %s: %s", name, s.Description))
	}

	result := fmt.Sprintf("Available Skills (%d):\n\n%s\n\nUse skill(action=\"get\", name=\"<name>\") to get full instructions.", len(t.skills), strings.Join(lines, "\n"))
	return &tool.ToolResult{Success: true, Data: result}
}

func (t *SkillTool) executeGet(args map[string]any) *tool.ToolResult {
	name, _ := args["name"].(string)
	if name == "" {
		return &tool.ToolResult{Success: false, Error: "name is required for action='get'"}
	}

	s, ok := t.skills[name]
	if !ok {
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("skill not found: %s", name)}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Skill: %s\n\n**Description**: %s\n\n%s", name, s.Description, s.Instructions)

	if len(s.ResourceFiles) > 0 {
		sb.WriteString("\n\n### Resource Files\n\n")
		for _, rf := range s.ResourceFiles {
			fmt.Fprintf(&sb, "- %s\n", rf.Name)
		}
	}

	return &tool.ToolResult{Success: true, Data: sb.String()}
}

func (t *SkillTool) executeSearch(args map[string]any) *tool.ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return &tool.ToolResult{Success: false, Error: "query is required for action='search'"}
	}

	queryLower := strings.ToLower(query)

	names := make([]string, 0, len(t.skills))
	for name := range t.skills {
		names = append(names, name)
	}
	sort.Strings(names)

	var matches []string
	for _, name := range names {
		s := t.skills[name]
		if strings.Contains(strings.ToLower(name), queryLower) ||
			strings.Contains(strings.ToLower(s.Description), queryLower) {
			matches = append(matches, fmt.Sprintf("- %s: %s", name, s.Description))
		}
	}

	if len(matches) == 0 {
		return &tool.ToolResult{Success: true, Data: fmt.Sprintf("No skills match query '%s'.", query)}
	}

	result := fmt.Sprintf("Available Skills (%d):\n\n%s\n\nUse skill(action=\"get\", name=\"<name>\") to get full instructions.", len(matches), strings.Join(matches, "\n"))
	return &tool.ToolResult{Success: true, Data: result}
}

var _ tool.Tool = (*SkillTool)(nil)
