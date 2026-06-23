package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionNameUpdatesStreamsGeneratedName(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Investigate naming updates")
	now := time.Now().UTC()
	if err := h.db.Model(&model.Session{}).
		Where("id = ?", created.Session.ID).
		Updates(map[string]any{
			"name":                           "generated-session-name",
			"session_name_auto_generated_at": now,
		}).Error; err != nil {
		t.Fatalf("mark session named: %v", err)
	}

	rr := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID+"/name-updates", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("name updates status=%d body=%s", rr.Code, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("content-type=%q, want event stream", contentType)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event: session.name") || !strings.Contains(body, `"name":"generated-session-name"`) {
		t.Fatalf("missing generated name event: %s", body)
	}
}
