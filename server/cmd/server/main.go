package main

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/copcon/core"
	"github.com/copcon/core/llm"
	knowledgebase "github.com/copcon/plugins/knowledge-base"
	"github.com/copcon/plugins/knowledge-base/sqlitevec"
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/config"
	stor "github.com/copcon/server/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load()
	chk(log, err)
	storeProvider, err := stor.CreateStoreProvider(cfg.Database)
	chk(log, err)
	cl := openai.NewClient(option.WithAPIKey(cfg.OpenAI.APIKey), option.WithBaseURL(cfg.OpenAI.BaseURL))

	h := core.NewHarness(core.HarnessConfig{
		Store:  core.StoreConfig{Provider: storeProvider},
		LLM:    llm.NewOpenAIAdapter(&cl, cfg.OpenAI.Model),
		Logger: log,
		Agents: agentSpecs(cfg),
	})
	chk(log, h.Build())

	// Create knowledge store explicitly if knowledge bases are configured
	var ks knowledgebase.KnowledgeStore
	if len(cfg.KnowledgeBases) > 0 {
		var ksErr error
		ks, ksErr = sqlitevec.NewKnowledgeStoreFromDSN(":memory:")
		if ksErr != nil {
			log.Warn("failed to create knowledge store", "error", ksErr)
		}
	}

	var apiOpts []api.HandlerOption
	apiOpts = append(apiOpts, api.BuildKnowledgeOptions(cfg, ks, llm.NewOpenAIAdapter(&cl, cfg.OpenAI.Model))...)

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	api.SetupRoutes(r, cfg, h, apiOpts...)
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