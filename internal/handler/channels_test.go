package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_ChannelsCreate_AllowsSameNameAcrossSources(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	base := map[string]any{
		"name":             "#engineering",
		"default_agent_id": fx.agent.ID.String(),
	}

	native := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, base)
	if native.Code != http.StatusCreated {
		t.Fatalf("native status=%d body=%s", native.Code, native.Body.String())
	}
	nativeOut := decodeChannelCreate(t, native)
	if nativeOut.Channel.Name != "engineering" || nativeOut.Channel.Origin != "native" {
		t.Fatalf("native channel source/name mismatch: %+v", nativeOut.Channel)
	}

	slack := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
		"external_workspace_key": "T123",
		"external_resource_type": "channel",
		"external_resource_key":  "CENG",
		"external_resource_name": "engineering",
		"external_resource_url":  "https://slack.test/T123/CENG",
		"external_metadata":      map[string]any{"is_private": false},
	})
	if slack.Code != http.StatusCreated {
		t.Fatalf("slack status=%d body=%s", slack.Code, slack.Body.String())
	}
	slackOut := decodeChannelCreate(t, slack)
	if slackOut.Channel.Origin != "external" || slackOut.Channel.ExternalProvider != "slack" {
		t.Fatalf("slack channel source mismatch: %+v", slackOut.Channel)
	}

	discord := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "discord",
		"external_workspace_key": "GUILD-1",
		"external_resource_key":  "987",
	})
	if discord.Code != http.StatusCreated {
		t.Fatalf("discord status=%d body=%s", discord.Code, discord.Body.String())
	}

	duplicateSlackName := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
		"external_workspace_key": "T123",
		"external_resource_key":  "COTHER",
	})
	if duplicateSlackName.Code != http.StatusConflict {
		t.Fatalf("duplicate slack same source/name status=%d body=%s", duplicateSlackName.Code, duplicateSlackName.Body.String())
	}

	duplicateSlackResource := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#platform",
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
		"external_workspace_key": "T123",
		"external_resource_key":  "CENG",
	})
	if duplicateSlackResource.Code != http.StatusConflict {
		t.Fatalf("duplicate slack resource status=%d body=%s", duplicateSlackResource.Code, duplicateSlackResource.Body.String())
	}

	var count int64
	if err := h.db.Model(&model.Channel{}).
		Where("org_id = ? AND name = ?", fx.org.ID, "engineering").
		Count(&count).Error; err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if count != 3 {
		t.Fatalf("engineering channel count=%d, want 3", count)
	}
}

func TestIntegration_ChannelsVisibilityAndJoin(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	publicID := createChannelForTest(t, h, fx, fx.owner, "ops", "public")
	privateID := createChannelForTest(t, h, fx, fx.owner, "leadership", "private")

	privateGet := h.doJSON(t, http.MethodGet, "/v1/channels/"+privateID, fx, fx.member, nil)
	if privateGet.Code != http.StatusForbidden {
		t.Fatalf("private get status=%d body=%s", privateGet.Code, privateGet.Body.String())
	}

	memberList := h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil)
	assertChannelNames(t, memberList, nil)
	discoverable := h.doJSON(t, http.MethodGet, "/v1/channels?discoverable=true", fx, fx.member, nil)
	assertChannelNames(t, discoverable, []string{"ops"})

	joined := h.doJSON(t, http.MethodPost, "/v1/channels/"+publicID+"/join", fx, fx.member, nil)
	if joined.Code != http.StatusOK {
		t.Fatalf("join status=%d body=%s", joined.Code, joined.Body.String())
	}
	memberList = h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil)
	assertChannelNames(t, memberList, []string{"ops"})
	if privateID == "" {
		t.Fatal("private channel was not created")
	}
}

func TestIntegration_ChannelsManagementGuardrails(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "platform", "public")

	memberPatch := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channelID, fx, fx.member, map[string]any{
		"description": "member attempted edit",
	})
	if memberPatch.Code != http.StatusForbidden {
		t.Fatalf("member patch status=%d body=%s", memberPatch.Code, memberPatch.Body.String())
	}

	ownerPatch := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channelID, fx, fx.owner, map[string]any{
		"name":        "platform-team",
		"description": "owner edit",
	})
	if ownerPatch.Code != http.StatusOK {
		t.Fatalf("owner patch status=%d body=%s", ownerPatch.Code, ownerPatch.Body.String())
	}
	out := decodeChannelCreate(t, ownerPatch)
	if out.Channel.Name != "platform-team" {
		t.Fatalf("patched name=%q", out.Channel.Name)
	}

	generalID := seedDefaultChannel(t, h, fx)
	deleteDefault := h.doJSON(t, http.MethodDelete, "/v1/channels/"+generalID, fx, fx.owner, nil)
	if deleteDefault.Code != http.StatusBadRequest {
		t.Fatalf("delete default status=%d body=%s", deleteDefault.Code, deleteDefault.Body.String())
	}
}

func createChannelForTest(t *testing.T, h *channelHarness, fx channelFixture, user model.User, name, visibility string) string {
	t.Helper()
	rr := h.doJSON(t, http.MethodPost, "/v1/channels", fx, user, map[string]any{
		"name":             name,
		"visibility":       visibility,
		"default_agent_id": fx.agent.ID.String(),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create channel %s status=%d body=%s", name, rr.Code, rr.Body.String())
	}
	out := decodeChannelCreate(t, rr)
	return out.Channel.ID
}

func assertChannelNames(t *testing.T, rr *httptest.ResponseRecorder, want []string) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out channelListOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
	}
	got := make([]string, len(out.Data))
	for i, channel := range out.Data {
		got[i] = channel.Name
	}
	if len(got) != len(want) {
		t.Fatalf("channel names=%v, want=%v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("channel names=%v, want=%v", got, want)
		}
	}
}

func seedDefaultChannel(t *testing.T, h *channelHarness, fx channelFixture) string {
	t.Helper()
	channel := model.Channel{
		OrgID:          fx.org.ID,
		Name:           "general-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: fx.agent.ID,
		IsDefault:      true,
		Origin:         "native",
		CreatedBy:      &fx.owner.ID,
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create default channel: %v", err)
	}
	member := model.ChannelMember{ChannelID: channel.ID, UserID: fx.owner.ID, Role: "owner", CreatedAt: time.Now()}
	if err := h.db.Create(&member).Error; err != nil {
		t.Fatalf("create default channel member: %v", err)
	}
	return channel.ID.String()
}
