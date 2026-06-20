package e2e

var agentRuntimeE2EHakareeSubagents = []string{
	"codebase-explorer",
	"librarian",
	"oracle",
}

var agentRuntimeE2ESubagentMarkers = map[string]string{
	"codebase-explorer": "CODEBASE_EXPLORER_SUBAGENT_CONFIRMED",
	"librarian":         "LIBRARIAN_SUBAGENT_CONFIRMED",
	"oracle":            "ORACLE_SUBAGENT_CONFIRMED",
}

func agentRuntimeE2EAllSubagentMarkers() []string {
	out := make([]string, 0, len(agentRuntimeE2EHakareeSubagents))
	for _, agent := range agentRuntimeE2EHakareeSubagents {
		out = append(out, agentRuntimeE2ESubagentMarkers[agent])
	}
	return out
}
