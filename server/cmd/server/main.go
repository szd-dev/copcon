package main

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/copcon/core"
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/capabilities/hooks"
	"github.com/copcon/core/capabilities/tools"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/copcon/plugins/embedding-openai"
	knowledgebase "github.com/copcon/plugins/knowledge-base"
	"github.com/copcon/plugins/knowledge-base/sqlitevec"
	memoryfile "github.com/copcon/plugins/memory-file"
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

	fmStore, fmErr := memoryfile.NewFileMemoryStore(
		defaultMemoryBasePath(),
		200,
		25*1024,
	)
	if fmErr != nil {
		log.Warn("failed to create file memory store", "error", fmErr)
	}

	ks, ksErr := sqlitevec.NewKnowledgeStoreFromDSN(knowledgeStoreDSN(cfg))
	if ksErr != nil {
		log.Warn("failed to create knowledge store", "error", ksErr)
	}

	llmAdapter := llm.NewOpenAIAdapter(&cl, cfg.OpenAI.Model)
	embCfg := resolveEmbeddingConfig(cfg)
	var emb storage.Embedder
	if embCfg.Backend != "" {
		emb, err = embedding.NewFromConfig(embCfg, llmAdapter)
		if err != nil {
			log.Warn("failed to create embedder", "error", err)
		}
	}

	reg := capabilities.NewRegistry()
	hooks.RegisterAll(reg)
	tools.RegisterAll(reg)

	if fmStore != nil {
		memoryfile.RegisterCapabilities(reg, fmStore)
	}
	if ks != nil {
		knowledgebase.RegisterCapabilities(reg, ks, emb, fmStore)
	}

	h := core.NewHarness(core.HarnessConfig{
		Registry: reg,
		Store:    core.StoreConfig{Provider: storeProvider},
		LLM:      llmAdapter,
		Logger:   log,
		Agents:   agentSpecs(cfg),
	})
	chk(log, h.Build())

	var apiOpts []api.HandlerOption
	if fmStore != nil {
		apiOpts = append(apiOpts, api.WithMemoryStore(fmStore))
	}
	apiOpts = append(apiOpts, api.BuildKnowledgeOptions(ks, emb)...)

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
			Memory: core.MemorySpec{
				Enabled:       a.Memory.Enabled,
				BasePath:      a.Memory.BasePath,
				SystemDir:     a.Memory.SystemDir,
				IndexFile:     a.Memory.IndexFile,
				MaxIndexLines: a.Memory.MaxIndexLines,
				MaxIndexBytes: a.Memory.MaxIndexBytes,
			},
			KnowledgeBases: a.KnowledgeBases,
		})
	}
	return out
}

func defaultMemoryBasePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir() + "/copcon-memory"
	}
	return home + "/.copcon/memory"
}

func resolveEmbeddingConfig(cfg *config.Config) embedding.EmbeddingConfig {
	if len(cfg.KnowledgeBases) == 0 {
		return embedding.EmbeddingConfig{}
	}

	kb := cfg.KnowledgeBases[0]
	if kb.Embedding.Backend == "" {
		return embedding.EmbeddingConfig{}
	}

	return embedding.EmbeddingConfig{
		Backend:     embedding.BackendType(kb.Embedding.Backend),
		BaseURL:     cfg.OpenAI.BaseURL,
		APIKey:      cfg.OpenAI.APIKey,
		OpenAIModel: kb.Embedding.OpenAIModel,
	}
}

func knowledgeStoreDSN(cfg *config.Config) string {
	for _, kb := range cfg.KnowledgeBases {
		if kb.SQLitePath != "" {
			return kb.SQLitePath
		}
	}
	return "file::memory:?cache=shared"
}
