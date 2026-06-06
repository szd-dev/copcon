package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"gorm.io/gorm"

	_ "modernc.org/sqlite/vec" // enable vec0 extension in modernc SQLite driver

	"github.com/copcon/core"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/plugin"
	kbembedding "github.com/copcon/plugins/knowledge-base/embedding"
	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
	"github.com/copcon/plugins/knowledge-base/store/bruteforce"
	"github.com/copcon/plugins/knowledge-base/store/sqlitevec"
	memoryfile "github.com/copcon/plugins/memory-file"
	"github.com/copcon/plugins/mcp"
	"github.com/copcon/plugins/skill"
	"github.com/copcon/server/internal/api"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/kbworker"
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

	ks, ksErr := createKnowledgeStore(cfg, log)
	if ksErr != nil {
		log.Warn("failed to create knowledge store", "error", ksErr)
	}

	llmAdapter := llm.NewOpenAIAdapter(&cl, cfg.OpenAI.Model)
	embCfg := resolveEmbeddingConfig(cfg)
	var emb kbtypes.Embedder
	if embCfg.Backend != "" {
		emb, err = kbembedding.NewFromConfig(embCfg, llmAdapter)
		if err != nil {
			log.Warn("failed to create embedder", "error", err)
		}
	}

	if ks != nil && emb != nil {
		worker := kbworker.New(ks, emb, 10*time.Second)
		worker.Start()
		defer worker.Stop()
	}

	h := core.NewHarness(core.HarnessConfig{
		Store:  core.StoreConfig{Provider: storeProvider},
		LLM:    llmAdapter,
		Logger: log,
		Agents: agentSpecs(cfg),
	})

	h.Register(plugin.NewBuiltin())

	var summaryLLM llm.LLMProvider
	if cfg.Memory.Summarization.Enabled {
		summaryCL := openai.NewClient(
			option.WithAPIKey(cfg.Memory.Summarization.APIKey),
			option.WithBaseURL(cfg.Memory.Summarization.BaseURL),
		)
		summaryLLM = llm.NewOpenAIAdapter(&summaryCL, cfg.Memory.Summarization.Model)
	}
	if summaryLLM == nil {
		summaryLLM = llmAdapter
	}

	if fmStore != nil {
		h.Register(memoryfile.NewPlugin(fmStore, llmAdapter, summaryLLM))
	}
	if ks != nil {
		h.Register(knowledgebase.NewPlugin(ks, emb))
	}

	if cfg.Skills.Enabled {
		h.Register(skill.NewPlugin(skill.Config{
			ProjectRoot: projectRoot(),
			ExtraPaths:  cfg.Skills.ExtraPaths,
		}))
	}

	if cfg.MCP.Enabled && len(cfg.MCP.Servers) > 0 {
		mcpConfigs := convertMCPServerConfigs(cfg.MCP.Servers)
		h.Register(mcp.NewPlugin(mcpConfigs))
	}

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
			ID:             a.ID,
			Name:           a.Name,
			Model:          a.Model,
			SystemPrompt:   a.SystemPrompt,
			Tools:          a.Tools,
			AllowDelegate:  a.AllowDelegate,
			KnowledgeBases: a.KnowledgeBases,
		})
	}
	return out
}

func projectRoot() string {
	root, err := filepath.Abs(".")
	if err != nil {
		return "."
	}
	return root
}

func convertMCPServerConfigs(servers []config.MCPServerConfig) []mcp.MCPServerConfig {
	out := make([]mcp.MCPServerConfig, 0, len(servers))
	for _, s := range servers {
		var allowed *mcp.AllowedToolsConfig
		if s.AllowedTools != nil {
			allowed = &mcp.AllowedToolsConfig{
				Include: s.AllowedTools.Include,
				Exclude: s.AllowedTools.Exclude,
			}
		}
		out = append(out, mcp.MCPServerConfig{
			Name:         s.Name,
			Type:         mcp.TransportType(s.Type),
			Command:      s.Command,
			Args:         s.Args,
			Env:          s.Env,
			URL:          s.URL,
			AllowedTools: allowed,
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

func resolveEmbeddingConfig(cfg *config.Config) kbembedding.EmbeddingConfig {
	k := cfg.Knowledge.Embedding
	return kbembedding.EmbeddingConfig{
		Backend:     kbembedding.BackendType(k.Backend),
		OpenAIModel: k.Model,
		BaseURL:     k.BaseURL,
		APIKey:      k.APIKey,
	}
}

func createKnowledgeStore(cfg *config.Config, log *slog.Logger) (*sqlitevec.KnowledgeStore, error) {
	path := cfg.Knowledge.SQLitePath
	if path == "" {
		path = "data/collab/knowledge.db"
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create knowledge store directory %s: %w", dir, err)
	}

	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)

	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite for knowledge store: %w", err)
	}

	var vec knowledgebase.VectorStore
	switch cfg.Knowledge.VectorBackend {
	case "sqlite-vec":
		sqlDB, err := gormDB.DB()
		if err != nil {
			return nil, fmt.Errorf("get underlying sql.DB: %w", err)
		}
		vecStore := sqlitevec.New(sqlDB, 1536)
		if err := vecStore.InitVectorTable(context.Background()); err != nil {
			return nil, fmt.Errorf("init vector table: %w", err)
		}
		vec = vecStore
		log.Info("using sqlite-vec vector backend for knowledge store")
	default:
		vec = bruteforce.New(gormDB)
		log.Info("using brute-force vector backend for knowledge store")
	}

	return sqlitevec.NewKnowledgeStore(gormDB, vec)
}
