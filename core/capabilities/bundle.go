package capabilities

// MemoryBundleNames returns the capability names that constitute the
// file-based memory bundle. These are auto-included when an AgentSpec
// has Memory.Enabled set to true.
func MemoryBundleNames() []string {
	return []string{
		HookMemory,
		CapMemoryFile,
	}
}

// KnowledgeBaseBundleNames returns the capability names that constitute the
// knowledge-base bundle. These are auto-included when an AgentSpec
// specifies one or more KnowledgeBases.
func KnowledgeBaseBundleNames() []string {
	return []string{
		HookKBRecall,
		HookMemoryPersist,
	}
}
