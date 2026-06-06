package skill

// Skill represents a parsed Skill definition following the Agent Skills Specification.
type Skill struct {
	// Metadata (from YAML frontmatter)
	Name         string            // skill name, must match parent directory name
	Description  string            // functional description + when to use
	License      string            // license name (optional)
	Metadata     map[string]string // arbitrary key-value pairs (optional)
	AllowedTools string            // space-separated list of pre-approved tools (optional)

	// Content (from Markdown body)
	Instructions string // SKILL.md body content

	// File system info
	DirPath       string         // absolute path to skill directory
	Source        string         // source path where discovered (for dedup tracking)
	ResourceFiles []ResourceFile // files under scripts/, references/, assets/
}

// ResourceFile represents a resource file inside a Skill directory.
type ResourceFile struct {
	Name     string // relative path from skill dir, e.g. "scripts/run.sh"
	Path     string // absolute filesystem path
	Category string // "scripts" | "references" | "assets"
}

// SkillSummary is a Level-1 disclosure summary (metadata only).
type SkillSummary struct {
	Name        string
	Description string
	Source      string // source path
}