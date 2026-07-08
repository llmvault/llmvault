package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/memory"
)

type channelCategoryOut struct {
	Channel struct {
		ID                   string `json:"id"`
		Category             string `json:"category"`
		MemoryMission        string `json:"memory_mission"`
		DefaultMemoryMission string `json:"default_memory_mission"`
	} `json:"channel"`
}

func decodeChannelCategory(t *testing.T, rr *httptest.ResponseRecorder) channelCategoryOut {
	t.Helper()
	var out channelCategoryOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode channel response: %v\n%s", err, rr.Body.String())
	}
	return out
}

func TestIntegration_ChannelsCreate_CategoryRequired(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)

	missing := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":             "no-category",
		"default_agent_id": fx.agent.ID.String(),
	})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing category status=%d body=%s", missing.Code, missing.Body.String())
	}

	invalid := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":             "bad-category",
		"category":         "space-lasers",
		"default_agent_id": fx.agent.ID.String(),
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid category status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestIntegration_ChannelsCreate_CategorySeedsMissionTemplate(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)

	create := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":             "support-queue",
		"category":         "customer-support",
		"team_id":          fx.team.ID.String(),
		"default_agent_id": fx.agent.ID.String(),
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	out := decodeChannelCategory(t, create)
	if out.Channel.Category != "customer-support" {
		t.Fatalf("category=%q, want customer-support", out.Channel.Category)
	}
	// The curated template is stored verbatim on create — deterministic, no
	// LLM specialization.
	if want := memory.MissionTemplate(memory.ChannelCategoryCustomerSupport); out.Channel.MemoryMission != want {
		t.Fatalf("memory_mission=%q, want template %q", out.Channel.MemoryMission, want)
	}
	// The category default rides along so the UI can always show the
	// effective mission, set or not.
	if want := memory.MissionTemplate(memory.ChannelCategoryCustomerSupport); out.Channel.DefaultMemoryMission != want {
		t.Fatalf("default_memory_mission=%q, want template %q", out.Channel.DefaultMemoryMission, want)
	}
}

func TestIntegration_ChannelsCreate_ExplicitMissionOverridesTemplate(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)

	create := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":             "acme-support",
		"category":         "customer-support",
		"memory_mission":   "Only retain ACME escalation rules.",
		"team_id":          fx.team.ID.String(),
		"default_agent_id": fx.agent.ID.String(),
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	out := decodeChannelCategory(t, create)
	if out.Channel.MemoryMission != "Only retain ACME escalation rules." {
		t.Fatalf("memory_mission=%q, want the explicit mission from the create body", out.Channel.MemoryMission)
	}
}

func TestIntegration_ChannelsCreate_GeneralHasNoMission(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)

	create := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":             "watercooler",
		"category":         "general",
		"team_id":          fx.team.ID.String(),
		"default_agent_id": fx.agent.ID.String(),
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	out := decodeChannelCategory(t, create)
	if out.Channel.Category != "general" || out.Channel.MemoryMission != "" {
		t.Fatalf("general channel category=%q mission=%q, want general/empty", out.Channel.Category, out.Channel.MemoryMission)
	}
}

func TestIntegration_ChannelsUpdate_CategoryAndMission(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "ops-memory", "public")

	invalid := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channelID, fx, fx.owner, map[string]any{
		"category": "nonsense",
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid category update status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	update := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channelID, fx, fx.owner, map[string]any{
		"category":       "operations",
		"memory_mission": "Track vendor changes only.",
	})
	if update.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}
	out := decodeChannelCategory(t, update)
	if out.Channel.Category != "operations" || out.Channel.MemoryMission != "Track vendor changes only." {
		t.Fatalf("update result category=%q mission=%q", out.Channel.Category, out.Channel.MemoryMission)
	}

	clear := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channelID, fx, fx.owner, map[string]any{
		"memory_mission": "",
	})
	if clear.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", clear.Code, clear.Body.String())
	}
	cleared := decodeChannelCategory(t, clear)
	if cleared.Channel.MemoryMission != "" {
		t.Fatalf("mission=%q, want cleared", cleared.Channel.MemoryMission)
	}
	// A cleared mission means the channel follows its category default, which
	// the response surfaces for the UI to display as the effective mission.
	if want := memory.MissionTemplate(memory.ChannelCategoryOperations); cleared.Channel.DefaultMemoryMission != want {
		t.Fatalf("default_memory_mission=%q, want operations template %q", cleared.Channel.DefaultMemoryMission, want)
	}
}
