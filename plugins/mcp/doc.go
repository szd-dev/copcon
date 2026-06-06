// Package mcp implements the MCP (Model Context Protocol) plugin for CopCon.
//
// It provides types and utilities for connecting to MCP servers, discovering
// MCP tools, and wrapping them as CopCon tool.Tool instances that the agent
// can invoke.
//
// Usage:
//
//	import "github.com/copcon/plugins/mcp"
//
//	cfg := mcp.MCPServerConfig{
//	    Name:    "my-server",
//	    Type:    mcp.TransportStdio,
//	    Command: "npx",
//	    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
//	}
package mcp