package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestCanvasArtifactPreviewBindsToSourceSession(t *testing.T) {
	h := newCanvasArtifactHarness(t)

	create := h.doRuntimeJSON(t, http.MethodPost, "/internal/agents/"+h.agentID.String()+"/canvas/projects", map[string]any{
		"name": "Bind pages",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", create.Code, create.Body.String())
	}
	var createdProject struct {
		Slug string `json:"slug"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &createdProject); err != nil {
		t.Fatalf("decode create project: %v", err)
	}

	sync := h.doRuntimeJSON(t, http.MethodPost, "/internal/agents/"+h.agentID.String()+"/canvas/artifacts/sync", map[string]any{
		"session_id": h.sessionID.String(),
		"project":    map[string]any{"id": createdProject.ID, "slug": createdProject.Slug, "name": "Bind pages"},
		"artifact": map[string]any{
			"slug": "bind-page", "type": "web_page", "name": "Bind page",
			"manifest": map[string]any{
				"schema_version": 1, "kind": "hivy.canvas.artifact", "type": "web_page",
				"files": []any{map[string]any{"path": "index.html", "role": "entrypoint"}},
			},
		},
		"files": []any{map[string]any{
			"path": "index.html", "role": "entrypoint",
			"content_type": "text/html; charset=utf-8",
			"content":      "<!doctype html><html><body>Hi</body></html>",
		}},
	})
	if sync.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", sync.Code, sync.Body.String())
	}
	var synced struct {
		Artifact struct {
			ID string `json:"id"`
		} `json:"artifact"`
	}
	if err := json.Unmarshal(sync.Body.Bytes(), &synced); err != nil {
		t.Fatalf("decode sync: %v", err)
	}

	otherAgent := seedSessionAgent(t, h.db, h.org.ID)
	otherChannel := model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "OtherCanvas-" + uuid.NewString()[:8], Kind: "standard", TeamID: otherAgent.TeamID, DefaultAgentID: otherAgent.ID}
	if err := h.db.Create(&otherChannel).Error; err != nil {
		t.Fatalf("create other channel: %v", err)
	}
	otherSandbox := model.Sandbox{ID: uuid.New(), OrgID: &h.org.ID, AgentID: &otherAgent.ID, Status: "running", RuntimeURL: "http://other-runtime.test", EncryptedRuntimeSecret: []byte("x")}
	if err := h.db.Create(&otherSandbox).Error; err != nil {
		t.Fatalf("create other sandbox: %v", err)
	}
	otherSession := model.Session{ID: uuid.New(), OrgID: h.org.ID, ChannelID: otherChannel.ID, AgentID: otherAgent.ID, SandboxID: &otherSandbox.ID, Status: "active"}
	if err := h.db.Create(&otherSession).Error; err != nil {
		t.Fatalf("create other session: %v", err)
	}
	t.Cleanup(func() {
		h.db.Delete(&model.Session{}, "id = ?", otherSession.ID)
		h.db.Delete(&model.Sandbox{}, "id = ?", otherSandbox.ID)
		h.db.Delete(&model.Channel{}, "id = ?", otherChannel.ID)
		h.db.Delete(&model.Agent{}, "id = ?", otherAgent.ID)
	})

	denied := h.doOrgJSON(t, http.MethodPost, "/v1/canvas/artifacts/"+synced.Artifact.ID+"/preview-url", map[string]any{
		"session_id": otherSession.ID.String(),
	})
	if denied.Code != http.StatusNotFound {
		t.Fatalf("cross-session preview status=%d body=%s, want 404", denied.Code, denied.Body.String())
	}

	ok := h.doOrgJSON(t, http.MethodPost, "/v1/canvas/artifacts/"+synced.Artifact.ID+"/preview-url", map[string]any{
		"session_id": h.sessionID.String(),
	})
	if ok.Code != http.StatusOK {
		t.Fatalf("same-session preview status=%d body=%s, want 200", ok.Code, ok.Body.String())
	}
}
