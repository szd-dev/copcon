package main

import (
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/memory"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/todo"
	"github.com/copcon/server/internal/tool"
	"github.com/copcon/server/internal/tools"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	dsn := buildDSN(cfg.Database)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect database: %v", err)
	}

	if err := db.AutoMigrate(&session.Session{}, &session.Message{}, &session.Todo{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	sessionMgr := session.NewSessionManager(db)
	todoMgr := todo.NewTodoManager(db)
	contextMgr := chat_context.NewContextManager(db)
	memoryMgr := memory.NewMemoryManager(nil, "agent_memory")

	toolRegistry := tool.NewToolRegistry()
	if err := toolRegistry.Register(tools.NewCodeExecutor()); err != nil {
		log.Fatalf("Failed to register code executor: %v", err)
	}
	if err := toolRegistry.Register(tools.NewShellExecutor()); err != nil {
		log.Fatalf("Failed to register shell executor: %v", err)
	}
	if err := toolRegistry.Register(tools.NewFileOps("")); err != nil {
		log.Fatalf("Failed to register file ops: %v", err)
	}
	if err := toolRegistry.Register(tools.NewTodoTool(todoMgr)); err != nil {
		log.Fatalf("Failed to register todo tool: %v", err)
	}
	log.Printf("Registered %d tools in registry", len(toolRegistry.List()))

	agentRegistry, err := agent.NewAgentRegistry(cfg, toolRegistry)
	if err != nil {
		log.Fatalf("Failed to create agent registry: %v", err)
	}
	log.Printf("Loaded %d agents", len(agentRegistry.List()))

	agentEngine := agent.NewAgentEngine(agentRegistry, sessionMgr, contextMgr, memoryMgr, todoMgr)

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api.SetupRoutes(r, cfg, sessionMgr, todoMgr, agentEngine, agentRegistry)

	log.Printf("Server starting on :%s", cfg.Server.Port)
	if err := r.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func buildDSN(cfg config.DatabaseConfig) string {
	return "host=" + cfg.Host + " port=" + strconv.Itoa(cfg.Port) + " user=" + cfg.User + " password=" + cfg.Password + " dbname=" + cfg.DBName + " sslmode=disable"
}
