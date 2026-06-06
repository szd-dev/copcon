package api

type SkillInfo struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Enabled      bool              `json:"enabled"`
	Source       string            `json:"source"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	AllowedTools string            `json:"allowed_tools,omitempty"`
}

type SkillDetail struct {
	SkillInfo
	Instructions   string             `json:"instructions"`
	ResourceFiles  []ResourceFileInfo `json:"resource_files,omitempty"`
}

type ResourceFileInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Category string `json:"category"`
}

type MCPServerInfo struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	URL          string            `json:"url,omitempty"`
	Enabled      bool              `json:"enabled"`
	Tools        []string          `json:"tools,omitempty"`
	AllowedTools *AllowedToolsInfo `json:"allowed_tools,omitempty"`
}

type AllowedToolsInfo struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type MCPServerCreateRequest struct {
	Name         string            `json:"name" binding:"required"`
	Type         string            `json:"type" binding:"required"`
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	URL          string            `json:"url,omitempty"`
	AllowedTools *AllowedToolsInfo `json:"allowed_tools,omitempty"`
}

type SkillProvider interface {
	ListSkills() ([]SkillInfo, error)
	GetSkill(name string) (*SkillDetail, error)
	SetSkillEnabled(name string, enabled bool) error
}

type MCPProvider interface {
	ListServers() ([]MCPServerInfo, error)
	GetServer(name string) (*MCPServerInfo, error)
	AddServer(req MCPServerCreateRequest) (*MCPServerInfo, error)
	RemoveServer(name string) error
	SetServerEnabled(name string, enabled bool) error
}

func WithSkillProvider(sp SkillProvider) HandlerOption {
	return func(h *Handler) { h.skillProvider = sp }
}

func WithMCPProvider(mp MCPProvider) HandlerOption {
	return func(h *Handler) { h.mcpProvider = mp }
}
