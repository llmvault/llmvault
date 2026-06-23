package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_ChannelsListRecentSessionsExcludesArchived(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "archive-recent", "public")
	channelUUID := uuid.MustParse(channelID)
	base := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)
	active := seedChannelRecentSession(t, h, fx, channelUUID, fx.owner.ID, nil, base)
	archived := seedChannelRecentSession(t, h, fx, channelUUID, fx.owner.ID, nil, base.Add(time.Hour))
	if err := h.db.Model(&model.Session{}).
		Where("id = ?", archived.ID).
		Update("status", "archived").Error; err != nil {
		t.Fatalf("archive seeded session: %v", err)
	}

	rr := h.doJSON(t, http.MethodGet, "/v1/channels?include=recent_sessions&recent_sessions_limit=5", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out channelListOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
	}
	var got []sessionOut
	for _, channel := range out.Data {
		if channel.ID == channelID {
			got = channel.RecentSessions
			break
		}
	}
	if len(got) != 1 || got[0].ID != active.ID.String() {
		t.Fatalf("recent sessions=%+v, want only %s", got, active.ID)
	}
}
