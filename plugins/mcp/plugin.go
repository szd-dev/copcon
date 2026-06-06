package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/tool"
)

// MCPPlugin implements plugin.Plugin for the MCP subsystem.
type MCPPlugin struct {
	configs        []MCPServerConfig
	enabledServers map[string]bool
	mgr            *ConnectionManager
	logger         *slog.Logger
	tools          []tool.Tool
}

var _ plugin.Plugin = (*MCPPlugin)(nil)

func NewPlugin(configs []MCPServerConfig) plugin.Plugin {
	return &MCPPlugin{
		configs: configs,
		mgr:     NewConnectionManager(),
	}
}

func NewPluginWithManager(configs []MCPServerConfig, mgr *ConnectionManager) plugin.Plugin {
	return &MCPPlugin{
		configs: configs,
		mgr:     mgr,
	}
}

func (p *MCPPlugin) Name() string { return "mcp" }

func (p *MCPPlugin) Tools() []tool.Tool {
	return p.tools
}

func (p *MCPPlugin) Hooks() []hook.Hook {
	return nil
}

func (p *MCPPlugin) Init(deps plugin.PluginDeps) error {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	p.logger = logger

	p.enabledServers = make(map[string]bool, len(p.configs))
	for _, cfg := range p.configs {
		p.enabledServers[cfg.Name] = true
	}

	p.tools = p.discoverTools()
	return nil
}

func (p *MCPPlugin) Servers() []MCPServerConfig {
	result := make([]MCPServerConfig, len(p.configs))
	copy(result, p.configs)
	return result
}

func (p *MCPPlugin) SetServerEnabled(name string, enabled bool) {
	p.enabledServers[name] = enabled
}

func (p *MCPPlugin) IsServerEnabled(name string) bool {
	return p.enabledServers[name]
}

func (p *MCPPlugin) AddServer(cfg MCPServerConfig) {
	p.configs = append(p.configs, cfg)
	p.enabledServers[cfg.Name] = true
}

func (p *MCPPlugin) RemoveServer(name string) error {
	for i, cfg := range p.configs {
		if cfg.Name == name {
			p.configs = append(p.configs[:i], p.configs[i+1:]...)
			delete(p.enabledServers, name)
			return nil
		}
	}
	return fmt.Errorf("mcp server %q not found", name)
}

func (p *MCPPlugin) RefreshTools() {
	p.tools = p.discoverTools()
}

func (p *MCPPlugin) discoverTools() []tool.Tool {
	var allTools []tool.Tool

	for _, cfg := range p.configs {
		if !p.enabledServers[cfg.Name] {
			continue
		}

		session, err := p.mgr.GetSession(cfg.Name)
		if err != nil {
			p.logger.Warn("mcp session not connected", "server", cfg.Name, "error", err)
			continue
		}

		ctx := context.Background()
		listResult, err := session.ListTools(ctx, nil)
		if err != nil {
			p.logger.Warn("mcp list tools failed", "server", cfg.Name, "error", err)
			continue
		}

		callFunc := buildMCPToolCallFunc(session)

		for _, mcpTool := range listResult.Tools {
			if !isToolAllowed(mcpTool.Name, cfg.AllowedTools) {
				continue
			}

			info := MCPToolInfo{
				Name:        mcpTool.Name,
				Description: mcpTool.Description,
				InputSchema: convertSchema(mcpTool.InputSchema),
				ServerName:  cfg.Name,
			}
			wrapper := NewMCPToolWrapper(info, callFunc)
			allTools = append(allTools, &mcpToolRenamingWrapper{
				Tool:    wrapper,
				newName: pluginToolName(wrapper.Name()),
			})
		}
	}

	return allTools
}

func (p *MCPPlugin) ConnectionManager() *ConnectionManager {
	return p.mgr
}

// mcpToolRenamingWrapper overrides Name() to follow the "mcp.tool.{server}__{tool}"
// convention while delegating everything else to the wrapped tool.
type mcpToolRenamingWrapper struct {
	tool.Tool
	newName string
}

func (w *mcpToolRenamingWrapper) Name() string { return w.newName }

// pluginToolName converts "mcp__{server}__{tool}" → "mcp.tool.{server}__{tool}".
func pluginToolName(original string) string {
	return "mcp.tool." + original[len("mcp__"):]
}
