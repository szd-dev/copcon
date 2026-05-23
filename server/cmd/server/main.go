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
	"github.com/copcon/core/llm"
	pgstore "github.com/copcon/core/providers/postgres"
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/config"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load()
	chk(log, err)
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	chk(log, err)
	pg := pgstore.NewStore(db)
	cl := openai.NewClient(option.WithAPIKey(cfg.OpenAI.APIKey), option.WithBaseURL(cfg.OpenAI.BaseURL))

	h := core.NewHarness(core.HarnessConfig{
		Store:  core.StoreConfig{Provider: pg},
		LLM:    llm.NewOpenAIAdapter(&cl, cfg.OpenAI.Model),
		Logger: log,
		Agents: agentSpecs(cfg),
	})
	chk(log, h.Build())

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	api.SetupRoutes(r, cfg, h)
	log.Info("Server starting", "port", cfg.Server.Port)
	chk(log, r.Run(":"+cfg.Server.Port))
}

func chk(l *slog.Logger, err error) {
	if err != nil {
		l.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func agentSpecs(cfg *config.Config) []core.AgentSpec {
	out := make([]core.AgentSpec, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		out = append(out, core.AgentSpec{
			ID: a.ID, Name: a.Name, Model: a.Model, SystemPrompt: a.SystemPrompt,
			Tools:         a.Tools,
			AllowDelegate: a.ID == "code-assistant",
		})
	}
	return out
}