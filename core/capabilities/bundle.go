package capabilities

// MemoryBundleNames returns the capability names that constitute the
// file-based memory bundle. These are auto-included when an AgentSpec
// has Memory.Enabled set to true.
func MemoryBundleNames() []string {
	return []string{
		"hooks.file_memory",
		"tools.memory_store",
		"tools.memory_recall",
		"tools.memory_forget",
	}
}

// KnowledgeBaseBundleNames returns the capability names that constitute the
// knowledge-base bundle. These are auto-included when an AgentSpec
// specifies one or more KnowledgeBases.
func KnowledgeBaseBundleNames() []string {
	return []string{
		"hooks.kb_recall",
		"hooks.memory_persist",
	}
}
