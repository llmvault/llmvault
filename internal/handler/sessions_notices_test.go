package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/runtimestream"
)

type noticeStreamRecorder struct {
	mu     sync.Mutex
	hdr    http.Header
	status int
	buf    strings.Builder
}

func (s *noticeStreamRecorder) Header() http.Header {
	if s.hdr == nil {
		s.hdr = http.Header{}
	}
	return s.hdr
}

func (s *noticeStreamRecorder) WriteHeader(code int) { s.status = code }

func (s *noticeStreamRecorder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *noticeStreamRecorder) Flush() {}

func (s *noticeStreamRecorder) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestSessionNoticesForwardsSessionNotices(t *testing.T) {
	rc := connectTestRedis(t)
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis is not available: %v", err)
	}
	store := runtimestream.NewStore(rc, 1)

	h := newSessionHarnessWith(t, func(sh *handler.SessionHandler) {
		sh.WithRuntimeStreamStore(store)
	})
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Investigate notices")
	sessionID := uuid.MustParse(created.Session.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+created.Session.ID+"/notices", nil).WithContext(ctx)
	req.Header.Set("X-Org-ID", fx.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: fx.owner.ID.String(),
		OrgID:  fx.org.ID.String(),
	})

	rec := &noticeStreamRecorder{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.router.ServeHTTP(rec, req)
	}()

	waitForSubscriber(t, rc, runtimestream.LiveChannel(sessionID.String()))

	if err := store.PublishNotice(ctx, sessionID, runtimestream.Notice{
		Type:  runtimestream.NoticeTypeArtifactSynced,
		OrgID: fx.org.ID,
		Data:  json.RawMessage(`{"artifact_id":"art-session-1"}`),
	}); err != nil {
		t.Fatalf("publish artifact notice: %v", err)
	}
	if err := store.PublishNotice(ctx, sessionID, runtimestream.Notice{
		Type:      runtimestream.NoticeTypeUsageUpdated,
		OrgID:     fx.org.ID,
		SessionID: &sessionID,
		Data:      json.RawMessage(`{"session_id":"` + sessionID.String() + `"}`),
	}); err != nil {
		t.Fatalf("publish usage notice: %v", err)
	}

	filtered, err := json.Marshal(runtimestream.LiveMessage{
		Kind:      runtimestream.LiveKindRuntime,
		SessionID: sessionID.String(),
		Event: &runtimestream.Event{
			SessionID:  sessionID.String(),
			RuntimeSeq: 1,
			EventType:  "token",
			Payload:    map[string]any{"marker": "SHOULD_NOT_APPEAR"},
		},
	})
	if err != nil {
		t.Fatalf("marshal runtime message: %v", err)
	}
	if err := rc.Publish(ctx, runtimestream.LiveChannel(sessionID.String()), string(filtered)).Err(); err != nil {
		t.Fatalf("publish runtime message: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		body := rec.body()
		if strings.Contains(body, "art-session-1") && strings.Contains(body, runtimestream.NoticeTypeUsageUpdated) {
			if strings.Contains(body, "SHOULD_NOT_APPEAR") {
				t.Fatalf("runtime message leaked into notices stream: %s", body)
			}
			if strings.Count(body, "event: session.notice") < 2 {
				t.Fatalf("expected two session.notice frames: %s", body)
			}
			cancel()
			<-done
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf("notices never arrived: %s", rec.body())
}

func waitForSubscriber(t *testing.T, rc *redis.Client, channel string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		counts, err := rc.PubSubNumSub(context.Background(), channel).Result()
		if err == nil && counts[channel] >= 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("subscriber never attached to %s", channel)
}
