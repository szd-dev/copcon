package main

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/context_builder"
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
	contextMgr := chat_context.NewContextManager(db, todoMgr, context_builder.New(), logger)

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
	logger.Info("Loaded agents", "count", len(agentRegistry.List()))

	agentEngine := agent.NewAgentEngine(agentRegistry, sessionMgr, contextMgr, asyncRegistry)

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
