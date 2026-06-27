package handler_test

import (
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_ExternalTeamChannelVisibleToOrgUsers(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	team := seedChannelTeam(t, h, fx, "External")
	channel := model.Channel{
		OrgID:                fx.org.ID,
		Name:                 "slack-external",
		Kind:                 "standard",
		Visibility:           "public",
		TeamID:               &team.ID,
		DefaultAgentID:       fx.agent.ID,
		Origin:               "external",
		ExternalProvider:     "slack",
		ExternalResourceType: "slack_channel",
		ExternalResourceKey:  "CEXTERNAL",
		ExternalMetadata:     model.JSON{},
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create external channel: %v", err)
	}

	assertChannelNameSet(t, h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil), []string{"slack-external"})
	got := h.doJSON(t, http.MethodGet, "/v1/channels/"+channel.ID.String(), fx, fx.member, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("external channel get status=%d body=%s", got.Code, got.Body.String())
	}
}
