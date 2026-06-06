package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/copcon/plugins/mcp"
	"github.com/copcon/server/internal/api"
	internalconfig "github.com/copcon/server/internal/config"
)

var _ api.MCPProvider = (*MCPManager)(nil)

type MCPManager struct {
	plugin     *mcp.MCPPlugin
	configPath string
	mu         sync.Mutex
}

func NewMCPManager(p *mcp.MCPPlugin, configPath string) *MCPManager {
	return &MCPManager{plugin: p, configPath: configPath}
}

func (m *MCPManager) ListServers() ([]api.MCPServerInfo, error) {
	servers := m.plugin.Servers()
	result := make([]api.MCPServerInfo, 0, len(servers))
	for _, s := range servers {
		result = append(result, m.toServerInfo(s))
	}
	return result, nil
}

func (m *MCPManager) GetServer(name string) (*api.MCPServerInfo, error) {
	for _, s := range m.plugin.Servers() {
		if s.Name == name {
			info := m.toServerInfo(s)
			return &info, nil
		}
	}
	return nil, fmt.Errorf("mcp server %q not found", name)
}

func (m *MCPManager) AddServer(req api.MCPServerCreateRequest) (*api.MCPServerInfo, error) {
	cfg := mcp.MCPServerConfig{
		Name:    req.Name,
		Type:    mcp.TransportType(req.Type),
		Command: req.Command,
		Args:    req.Args,
		URL:     req.URL,
	}
	if req.AllowedTools != nil {
		cfg.AllowedTools = &mcp.AllowedToolsConfig{
			Include: req.AllowedTools.Include,
			Exclude: req.AllowedTools.Exclude,
		}
	}

	m.plugin.AddServer(cfg)
	if err := m.persistConfig(); err != nil {
		return nil, fmt.Errorf("persist config: %w", err)
	}

	info := m.toServerInfo(cfg)
	return &info, nil
}

func (m *MCPManager) RemoveServer(name string) error {
	if err := m.plugin.RemoveServer(name); err != nil {
		return err
	}
	if err := m.persistConfig(); err != nil {
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}

func (m *MCPManager) SetServerEnabled(name string, enabled bool) error {
	found := false
	for _, s := range m.plugin.Servers() {
		if s.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("mcp server %q not found", name)
	}
	m.plugin.SetServerEnabled(name, enabled)
	if err := m.persistConfig(); err != nil {
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}

func (m *MCPManager) toServerInfo(cfg mcp.MCPServerConfig) api.MCPServerInfo {
	info := api.MCPServerInfo{
		Name:    cfg.Name,
		Type:    string(cfg.Type),
		Command: cfg.Command,
		Args:    cfg.Args,
		URL:     cfg.URL,
		Enabled: m.plugin.IsServerEnabled(cfg.Name),
	}
	if cfg.AllowedTools != nil {
		info.AllowedTools = &api.AllowedToolsInfo{
			Include: cfg.AllowedTools.Include,
			Exclude: cfg.AllowedTools.Exclude,
		}
	}
	return info
}

func (m *MCPManager) persistConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var cfg internalconfig.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	existingServers := make(map[string]internalconfig.MCPServerConfig)
	for _, s := range cfg.MCP.Servers {
		existingServers[s.Name] = s
	}

	newServers := make([]internalconfig.MCPServerConfig, 0)
	for _, s := range m.plugin.Servers() {
		ic := internalconfig.MCPServerConfig{
			Name:    s.Name,
			Type:    string(s.Type),
			Command: s.Command,
			Args:    s.Args,
			URL:     s.URL,
		}
		if existing, ok := existingServers[s.Name]; ok {
			ic.Env = existing.Env
		}
		if s.AllowedTools != nil {
			ic.AllowedTools = &internalconfig.MCPAllowedTools{
				Include: s.AllowedTools.Include,
				Exclude: s.AllowedTools.Exclude,
			}
		}
		newServers = append(newServers, ic)
	}
	cfg.MCP.Servers = newServers

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(m.configPath)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, m.configPath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}
