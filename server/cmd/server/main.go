package main

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/core"
	"github.com/copcon/core/context_builder"
	"github.com/copcon/core/llm"
	pgstore "github.com/copcon/core/providers/postgres"
	"github.com/copcon/core/tool"
	"github.com/copcon/server/internal/adapter"
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/chat_context"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/tools/todo"
)

var toolToCap = map[string]string{
	"code_executor": "tools.code_executor", "shell_executor": "tools.shell_executor",
	"file_ops": "tools.file_ops", "todolist": "tools.todo",
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load(); chk(log, err)
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{}); chk(log, err)
	chk(log, db.AutoMigrate(&session.Session{}, &session.Message{}, &session.Todo{}))
	ar := tool.NewAsyncToolRegistry()
	sm := session.NewSessionManager(db, ar)
	tm, _ := todo.NewTodoManager(db)
	cm, ms := chat_context.NewContextManager(db, context_builder.New(), log)
	pg := pgstore.NewStore(db)
	cl := openai.NewClient(option.WithAPIKey(cfg.OpenAI.APIKey), option.WithBaseURL(cfg.OpenAI.BaseURL))
	h := core.NewHarness(core.HarnessConfig{
		Store: core.StoreConfig{Session: pg.SessionStore, Message: pg.MessageStore, Todo: pg.TodoStore},
		LLM: llm.NewOpenAIAdapter(&cl, cfg.OpenAI.Model), Logger: log,
		SessionManager: adapter.NewSessionManagerAdapter(sm), ContextManager: adapter.NewContextManagerAdapter(cm),
		AsyncTracker: ar, Agents: agentSpecs(cfg),
	}); chk(log, h.Build())
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	api.SetupRoutes(r, cfg, sm, tm, h.Engine(), h.Registry(), ms)
	log.Info("Server starting", "port", cfg.Server.Port)
	chk(log, r.Run(":"+cfg.Server.Port))
}

func chk(l *slog.Logger, err error) { if err != nil { l.Error("fatal", "error", err); os.Exit(1) } }

func agentSpecs(cfg *config.Config) []core.AgentSpec {
	hooks := []string{"hooks.todo_injection", "hooks.memory", "hooks.logging", "hooks.tracing"}
	out := make([]core.AgentSpec, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		caps := []string{"tools.hitl"}
		for _, t := range a.Tools { if c, ok := toolToCap[t]; ok { caps = append(caps, c) } }
		out = append(out, core.AgentSpec{ID: a.ID, Name: a.Name, Model: a.Model, SystemPrompt: a.SystemPrompt,
			Tools: caps, Hooks: hooks, AllowDelegate: a.ID == "code-assistant"})
	}
	return out
}
