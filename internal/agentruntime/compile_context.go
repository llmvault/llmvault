package agentruntime

func emptyMemoryContext() MemoryContext {
	return MemoryContext{Entries: []MemoryContextEntry{}, TokenBudget: 1000}
}
