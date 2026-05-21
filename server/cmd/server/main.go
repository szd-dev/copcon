package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/context_builder"
	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/llm"
	"github.com/copcon/server/internal/memory"
	"github.com/copcon/server/internal/plugins"
	"github.com/copcon/server/internal/plugins/logging"
	memoryplugin "github.com/copcon/server/internal/plugins/memory"
	"github.com/copcon/server/internal/plugins/tracing"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tool"
	"github.com/copcon/server/internal/tools"
	"github.com/copcon/server/internal/tools/todo"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	dsn := buildDSN(cfg.Database)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Error("Failed to connect database", "error", err)
		os.Exit(1)
	}

	if err := db.AutoMigrate(&session.Session{}, &session.Message{}, &session.Todo{}); err != nil {
		logger.Error("Failed to migrate database", "error", err)
		os.Exit(1)
	}

	// Create async tool registry first (shared between session manager and agent engine)
	asyncRegistry := tool.NewAsyncToolRegistry()

	sessionMgr := session.NewSessionManager(db, asyncRegistry)
	todoMgr := todo.NewTodoManager(db)
	contextMgr := chat_context.NewContextManager(db, context_builder.New(), logger)

	// Memory plugin uses nil Qdrant client (no-op until Qdrant is configured)
	memoryMgr := memory.NewMemoryManager(nil, "copcon")

	hookRunner := hook.NewHookRunner()
	hookRunner.Register(plugins.NewTodoInjectionHook(todoMgr))
	hookRunner.Register(memoryplugin.NewMemoryPlugin(memoryMgr))
	hookRunner.Register(logging.NewLoggingPlugin())
	hookRunner.Register(tracing.NewTracingPlugin(nil))

	toolRegistry := tool.NewToolRegistry()
	if err := toolRegistry.Register(tools.NewCodeExecutor()); err != nil {
		logger.Error("Failed to register code executor", "error", err)
		os.Exit(1)
	}
	if err := toolRegistry.Register(tools.NewShellExecutor()); err != nil {
		logger.Error("Failed to register shell executor", "error", err)
		os.Exit(1)
	}
	if err := toolRegistry.Register(tools.NewFileOps("")); err != nil {
		logger.Error("Failed to register file ops", "error", err)
		os.Exit(1)
	}
	if err := toolRegistry.Register(tools.NewTodoTool(todoMgr)); err != nil {
		logger.Error("Failed to register todo tool", "error", err)
		os.Exit(1)
	}
	if err := toolRegistry.Register(tools.NewGetToolStatusTool(asyncRegistry)); err != nil {
		logger.Error("Failed to register get_tool_status", "error", err)
		os.Exit(1)
	}
	if err := toolRegistry.Register(tools.NewGetToolResultTool(asyncRegistry)); err != nil {
		logger.Error("Failed to register get_tool_result", "error", err)
		os.Exit(1)
	}
	if err := toolRegistry.Register(tools.NewCancelToolTool(asyncRegistry)); err != nil {
		logger.Error("Failed to register cancel_tool", "error", err)
		os.Exit(1)
	}
	if err := toolRegistry.Register(tools.NewListAsyncToolsTool(asyncRegistry)); err != nil {
		logger.Error("Failed to register list_async_tools", "error", err)
		os.Exit(1)
	}
	logger.Info("Registered tools in registry", "count", len(toolRegistry.List()))

	agentRegistry, err := agent.NewAgentRegistry(cfg, toolRegistry)
	if err != nil {
		logger.Error("Failed to create agent registry", "error", err)
		os.Exit(1)
	}

	// Register built-in agent factories (overrides config-registered agents)
	// This is the migration path from config.yaml to code registration.
	for _, ac := range cfg.Agents {
		switch ac.ID {
		case "code-assistant", "chat-assistant":
			ac := ac
			allowDelegate := ac.ID == "code-assistant"
			agentRegistry.RegisterFactory(ac.ID, ac.Name, ac.Model, allowDelegate, func(ctx context.Context, params agent.CreateParams) (agent.AgentDefinition, error) {
				model := ac.Model
				if params.ModelOverride != "" {
					model = params.ModelOverride
				}

				toolMgr := tool.NewToolManager()
				for _, toolName := range ac.Tools {
					t, err := toolRegistry.Get(toolName)
					if err != nil {
						return agent.AgentDefinition{}, fmt.Errorf("agent %s: failed to get tool %s: %w", ac.ID, toolName, err)
					}
					if err := toolMgr.Register(t); err != nil {
						return agent.AgentDefinition{}, fmt.Errorf("agent %s: failed to register tool %s: %w", ac.ID, toolName, err)
					}
				}

				if ac.ID == "code-assistant" {
					if t, err := toolRegistry.Get("delegate_to"); err == nil {
						if err := toolMgr.Register(t); err != nil {
							return agent.AgentDefinition{}, fmt.Errorf("agent %s: failed to register delegate_to: %w", ac.ID, err)
						}
					}
					if t, err := toolRegistry.Get("read_sub_session"); err == nil {
						if err := toolMgr.Register(t); err != nil {
							return agent.AgentDefinition{}, fmt.Errorf("agent %s: failed to register read_sub_session: %w", ac.ID, err)
						}
					}
				}

				opts := []option.RequestOption{
					option.WithAPIKey(cfg.OpenAI.APIKey),
				}
				baseURL := ac.BaseURL
				if baseURL == "" {
					baseURL = cfg.OpenAI.BaseURL
				}
				if baseURL != "" {
					opts = append(opts, option.WithBaseURL(baseURL))
				}
				client := openai.NewClient(opts...)
				provider := llm.NewOpenAIAdapter(&client, model)

				systemPrompt := ac.SystemPrompt
				if params.Task != "" {
					systemPrompt = systemPrompt + "\n\nCurrent Task: " + params.Task
				}

				return agent.AgentDefinition{
					ID:           ac.ID,
					Name:         ac.Name,
					Model:        model,
					SystemPrompt: systemPrompt,
					ToolManager:  toolMgr,
					LLMProvider:  provider,
				}, nil
			})
		}
	}

	logger.Info("Loaded agents", "count", len(agentRegistry.List()))

	agentEngine := agent.NewAgentEngine(agentRegistry, sessionMgr, contextMgr, asyncRegistry, agent.WithHookRunner(hookRunner))

	// Register delegate_to tool (needs engine reference, done after engine creation)
	delegateTool := tools.NewDelegateToTool(agentRegistry, sessionMgr, contextMgr, agentEngine)
	if err := toolRegistry.Register(delegateTool); err != nil {
		logger.Error("Failed to register delegate_to tool", "error", err)
		os.Exit(1)
	}
	readSubSessionTool := tools.NewReadSubSessionTool(sessionMgr, contextMgr)
	if err := toolRegistry.Register(readSubSessionTool); err != nil {
		logger.Error("Failed to register read_sub_session tool", "error", err)
		os.Exit(1)
	}

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api.SetupRoutes(r, cfg, sessionMgr, todoMgr, agentEngine, agentRegistry)

	logger.Info("Server starting", "port", cfg.Server.Port)
	if err := r.Run(":" + cfg.Server.Port); err != nil {
		logger.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}

func buildDSN(cfg config.DatabaseConfig) string {
	return "host=" + cfg.Host + " port=" + strconv.Itoa(cfg.Port) + " user=" + cfg.User + " password=" + cfg.Password + " dbname=" + cfg.DBName + " sslmode=disable"
}
