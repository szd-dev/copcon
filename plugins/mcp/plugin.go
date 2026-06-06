package mcp

import (
	"context"
	"log/slog"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/tool"
)

// mcpPlugin implements plugin.Plugin for the MCP subsystem.
type mcpPlugin struct {
	configs []MCPServerConfig
	mgr     *ConnectionManager
	logger  *slog.Logger
	tools   []tool.Tool
}

var _ plugin.Plugin = (*mcpPlugin)(nil)

// NewPlugin creates a new MCP plugin with the given server configurations.
func NewPlugin(configs []MCPServerConfig) plugin.Plugin {
	return &mcpPlugin{
		configs: configs,
		mgr:     NewConnectionManager(),
	}
}

// NewPluginWithManager creates a new MCP plugin with a pre-configured
// ConnectionManager (useful for testing).
func NewPluginWithManager(configs []MCPServerConfig, mgr *ConnectionManager) plugin.Plugin {
	return &mcpPlugin{
		configs: configs,
		mgr:     mgr,
	}
}

func (p *mcpPlugin) Name() string { return "mcp" }

// Tools returns discovered MCP tools. Returns an empty slice before Init is called.
func (p *mcpPlugin) Tools() []tool.Tool {
	return p.tools
}

// Hooks returns nil — MCP plugins produce only tools.
func (p *mcpPlugin) Hooks() []hook.Hook {
	return nil
}

// Init injects dependencies and discovers MCP tools from all configured servers.
func (p *mcpPlugin) Init(deps plugin.PluginDeps) error {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	p.logger = logger

	p.tools = p.discoverTools()
	return nil
}

func (p *mcpPlugin) discoverTools() []tool.Tool {
	var allTools []tool.Tool

	for _, cfg := range p.configs {
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

// ConnectionManager returns the underlying ConnectionManager so callers
// can establish MCP sessions before Init is called.
func (p *mcpPlugin) ConnectionManager() *ConnectionManager {
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
