package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIntegration_ChannelsListRecentSessionsIncludeAllSourcesForMembers(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "slack-work", "public")
	channelUUID := uuid.MustParse(channelID)
	base := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)
	memberSession := seedChannelRecentSession(t, h, fx, channelUUID, fx.member.ID, nil, base.Add(time.Hour))
	slackSession := seedChannelSlackRecentSession(t, h, fx, channelUUID, base.Add(2*time.Hour))

	rr := h.doJSON(t, http.MethodGet, "/v1/channels?include=recent_sessions&recent_sessions_limit=5", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out channelListOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
	}
	var channel channelOut
	for _, entry := range out.Data {
		if entry.ID == channelID {
			channel = entry
			break
		}
	}
	if len(channel.RecentSessions) != 2 ||
		channel.RecentSessions[0].ID != slackSession.ID.String() ||
		channel.RecentSessions[1].ID != memberSession.ID.String() {
		t.Fatalf("recent sessions=%+v, want [%s %s]", channel.RecentSessions, slackSession.ID, memberSession.ID)
	}
}
