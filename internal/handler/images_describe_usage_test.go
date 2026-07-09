package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestImageDescribe_EnqueuesModelUsageForSession(t *testing.T) {
	h := newImageDescribeHarness(t)
	channel := model.Channel{
		OrgID:          h.org.ID,
		Name:           "image-usage-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		TeamID:         h.agent.TeamID,
		DefaultAgentID: h.agent.ID,
		Origin:         "native",
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	session := model.Session{
		OrgID:             h.org.ID,
		ChannelID:         channel.ID,
		AgentID:           h.agent.ID,
		SandboxID:         &h.sandbox.ID,
		Model:             h.agent.Model,
		ReasoningEffort:   "high",
		Source:            "web",
		SourceResourceKey: uuid.NewString(),
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := h.db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("session_id = ?", session.ID).Delete(&model.SessionEvent{})
		h.db.Where("id = ?", session.ID).Delete(&model.Session{})
		h.db.Where("id = ?", channel.ID).Delete(&model.Channel{})
	})

	body := `{"drive_asset_id":"` + h.asset.ID.String() + `","detail_level":"high","session_id":"` + session.ID.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/images/describe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var usageTask *tasks.ModelUsageWritePayload
	for _, task := range h.enqueuer.Tasks() {
		if task.TypeName != tasks.TypeModelUsageWrite {
			continue
		}
		var payload tasks.ModelUsageWritePayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			t.Fatalf("decode model usage payload: %v", err)
		}
		usageTask = &payload
	}
	if usageTask == nil {
		t.Fatalf("expected model usage task")
	}
	if usageTask.Generation.ProviderID != "openrouter" || usageTask.Generation.Model != "gemini-3.5-flash" {
		t.Fatalf("unexpected generation provider/model: %+v", usageTask.Generation)
	}
	if !usageTask.Generation.IsSystem {
		t.Fatalf("generation IsSystem = false, want true")
	}
	if usageTask.SessionEvent == nil || usageTask.SessionEvent.EventType != "model_usage" {
		t.Fatalf("missing model_usage session event: %+v", usageTask.SessionEvent)
	}
	if usageTask.SessionEvent.SessionID != session.ID {
		t.Fatalf("session event session = %s, want %s", usageTask.SessionEvent.SessionID, session.ID)
	}
}
