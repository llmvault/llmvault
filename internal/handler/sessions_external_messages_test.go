package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionsSendRejectsExternalChannelSession(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	session := seedSlackActivitySession(t, h, fx, time.Now())
	sourceKey := "slack:33c95935-9db7-4b14-aaab-681902ba5522:T0B57ABFZD5:C0B5KCLRSF7:1782596616.040029"
	if err := h.db.Model(&model.Session{}).
		Where("id = ?", session.ID).
		Update("source_resource_key", sourceKey).Error; err != nil {
		t.Fatalf("set source key: %v", err)
	}

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions/"+session.ID.String()+"/messages", fx, fx.bystander, map[string]any{
		"text": "continue from web",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("message status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error: %v\n%s", err, rr.Body.String())
	}
	if !strings.Contains(out.Error, "external provider Slack") || !strings.Contains(out.Error, "continue the conversation") {
		t.Fatalf("error=%q, want external Slack continuation message", out.Error)
	}
}
