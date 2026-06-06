package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	gmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ConnectionManager struct {
	mu       sync.RWMutex
	sessions map[string]*gmcp.ClientSession
}

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{sessions: make(map[string]*gmcp.ClientSession)}
}

func (m *ConnectionManager) Connect(ctx context.Context, cfg MCPServerConfig) (*gmcp.ClientSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[cfg.Name]; exists {
		return nil, fmt.Errorf("server already connected: %s", cfg.Name)
	}

	transport, err := createTransport(cfg)
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}

	client := gmcp.NewClient(&gmcp.Implementation{Name: "copcon-mcp-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", cfg.Name, err)
	}

	m.sessions[cfg.Name] = session
	return session, nil
}

func (m *ConnectionManager) ConnectWithTransport(ctx context.Context, name string, transport gmcp.Transport) (*gmcp.ClientSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[name]; exists {
		return nil, fmt.Errorf("server already connected: %s", name)
	}

	client := gmcp.NewClient(&gmcp.Implementation{Name: "copcon-mcp-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", name, err)
	}

	m.sessions[name] = session
	return session, nil
}

func createTransport(cfg MCPServerConfig) (gmcp.Transport, error) {
	switch cfg.Type {
	case TransportStdio:
		if cfg.Command == "" {
			return nil, fmt.Errorf("stdio transport requires command")
		}
		cmd := exec.Command(cfg.Command, cfg.Args...)
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		return &gmcp.CommandTransport{Command: cmd}, nil
	case TransportSSE:
		if cfg.URL == "" {
			return nil, fmt.Errorf("SSE transport requires URL")
		}
		return &gmcp.SSEClientTransport{Endpoint: cfg.URL}, nil
	case TransportStreamableHTTP:
		if cfg.URL == "" {
			return nil, fmt.Errorf("streamable-http transport requires URL")
		}
		return &gmcp.StreamableClientTransport{Endpoint: cfg.URL}, nil
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", cfg.Type)
	}
}

func (m *ConnectionManager) GetSession(name string) (*gmcp.ClientSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[name]
	if !ok {
		return nil, fmt.Errorf("server not connected: %s", name)
	}
	return session, nil
}

func (m *ConnectionManager) Disconnect(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[name]
	if !ok {
		return fmt.Errorf("server not connected: %s", name)
	}
	session.Close()
	delete(m.sessions, name)
	return nil
}

func (m *ConnectionManager) DisconnectAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, session := range m.sessions {
		session.Close()
		delete(m.sessions, name)
	}
}

func (m *ConnectionManager) ListSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.sessions))
	for name := range m.sessions {
		names = append(names, name)
	}
	return names
}
