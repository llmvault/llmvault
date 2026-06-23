package e2e

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestAgentProductionOpenWeightGauntletE2E(t *testing.T) {
	loadEnv(t)
	if os.Getenv("HIVY_AGENT_PRODUCTION_E2E") != "1" {
		t.Skip("set HIVY_AGENT_PRODUCTION_E2E=1 to run the production open-weight gauntlet")
	}

	models := productionE2EModels()
	runs := productionE2ERuns()
	mode := strings.TrimSpace(os.Getenv("HIVY_AGENT_PRODUCTION_E2E_MODE"))
	if mode == "" {
		mode = "smoke"
	}
	if mode == "smoke" && len(models) > 1 {
		models = models[:1]
	}
	if mode == "smoke" && runs > 1 {
		runs = 1
	}

	for _, modelID := range models {
		for run := 1; run <= runs; run++ {
			t.Run(safeTestName(modelID)+"/run-"+strconv.Itoa(run), func(t *testing.T) {
				t.Setenv("HIVY_AGENT_RUNTIME_E2E", "1")
				t.Setenv("HIVY_AGENT_RUNTIME_E2E_MODEL", modelID)
				t.Setenv("HIVY_AGENT_RUNTIME_E2E_TRACE_COMPACT", "1")
				TestAgentRuntimeCodingTaskE2E(t)
			})
		}
	}
}

func TestAgentProductionFullStackSessionE2E(t *testing.T) {
	loadEnv(t)
	if os.Getenv("HIVY_AGENT_PRODUCTION_E2E") != "1" {
		t.Skip("set HIVY_AGENT_PRODUCTION_E2E=1 to run the production full-stack session E2E")
	}
	t.Setenv("HIVY_AGENT_SESSIONS_E2E", "1")
	TestAgentSessionsDefaultGeneralChannelE2E(t)
}

func productionE2EModels() []string {
	raw := strings.TrimSpace(os.Getenv("HIVY_AGENT_PRODUCTION_E2E_MODELS"))
	if raw == "" {
		raw = "deepseek/deepseek-v4-flash"
	}
	items := strings.Split(raw, ",")
	models := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			models = append(models, item)
		}
	}
	if len(models) == 0 {
		models = []string{"deepseek/deepseek-v4-flash"}
	}
	return models
}

func productionE2ERuns() int {
	raw := strings.TrimSpace(os.Getenv("HIVY_AGENT_PRODUCTION_E2E_RUNS"))
	if raw == "" {
		return 1
	}
	runs, err := strconv.Atoi(raw)
	if err != nil || runs < 1 {
		return 1
	}
	if runs > 10 {
		return 10
	}
	return runs
}

func safeTestName(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("/", "_", ":", "_", ".", "_", " ", "_")
	return replacer.Replace(value)
}
