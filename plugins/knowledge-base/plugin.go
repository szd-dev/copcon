package knowledgebase

import (
	"github.com/copcon/core/hook"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/tool"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

// hooksNameWrapper overrides the Name() method of an embedded hook.Hook.
// This allows the plugin system to apply a namespaced name (e.g.
// "knowledge.hook.kb_recall") while preserving the original hook behavior.
type hooksNameWrapper struct {
	hook.Hook
	newName string
}

func (w *hooksNameWrapper) Name() string { return w.newName }

// kbPlugin implements plugin.Plugin for the knowledge-base plugin.
type kbPlugin struct {
	ks       KnowledgeStore
	emb      kbtypes.Embedder
	hook     hook.Hook
	agentKBs map[string][]string
}

// NewPlugin creates a knowledge-base Plugin with the given store and embedder.
// The store and embedder are required at construction time; AgentKnowledgeBases
// are injected later via Init.
func NewPlugin(ks KnowledgeStore, emb kbtypes.Embedder) plugin.Plugin {
	return &kbPlugin{
		ks:  ks,
		emb: emb,
	}
}

func (p *kbPlugin) Name() string { return "knowledge" }

func (p *kbPlugin) Tools() []tool.Tool { return nil }

func (p *kbPlugin) Hooks() []hook.Hook {
	h := NewKBRecallHook(p.emb, p.ks, p.agentKBs)
	p.hook = &hooksNameWrapper{Hook: h, newName: "knowledge.hook.kb_recall"}
	return []hook.Hook{p.hook}
}

func (p *kbPlugin) Init(deps plugin.PluginDeps) error {
	p.agentKBs = deps.AgentKnowledgeBases
	return nil
}

// GetStore exposes the underlying KnowledgeStore for API-layer usage.
func (p *kbPlugin) GetStore() KnowledgeStore { return p.ks }

// GetEmbedder exposes the underlying Embedder (optional, for API-layer usage).
func (p *kbPlugin) GetEmbedder() kbtypes.Embedder { return p.emb }
