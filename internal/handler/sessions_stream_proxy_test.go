package handler_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestSessionStreamRelaysPrivateRuntimeSSEImmediately(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	team := seedTeam(t, db, org.ID, "stream-proxy")
	agent := seedTeamAgent(t, db, org.ID, team.ID)
	encKey := testSymmetricKey(t)
	runtimeSecret := "runtime-stream-" + uuid.NewString()
	sessionID := uuid.New()
	encryptedSecret, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}

	releaseRuntime := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRuntime) }) }
	t.Cleanup(release)
	runtimeErrors := make(chan error, 1)
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/sessions/"+sessionID.String()+"/stream"; got != want {
			runtimeErrors <- fmt.Errorf("runtime path = %q, want %q", got, want)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+runtimeSecret {
			runtimeErrors <- fmt.Errorf("runtime authorization = %q", got)
			return
		}
		if got := r.URL.Query().Get("after_seq"); got != "7" {
			runtimeErrors <- fmt.Errorf("runtime after_seq = %q, want 7", got)
			return
		}
		if got := r.URL.Query().Get("ignored"); got != "" {
			runtimeErrors <- fmt.Errorf("unexpected query parameter forwarded: %q", got)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Hivy-Stream-Id", "runtime-stream-1")
		w.Header().Set("X-Hivy-Stream-Next-Sequence", "8")
		_, _ = fmt.Fprint(w, "event: token\ndata: {\"text\":\"first\",\"sequence\":8}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-releaseRuntime:
		case <-r.Context().Done():
			return
		}
		_, _ = fmt.Fprint(w, "event: turn_completed\ndata: {\"sequence\":9}\n\n")
	}))
	t.Cleanup(runtimeServer.Close)

	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		ProviderID:             "docker",
		ExternalID:             "stream-proxy-" + uuid.NewString(),
		RuntimeURL:             runtimeServer.URL,
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.Session{
		ID:              sessionID,
		OrgID:           org.ID,
		TeamID:          team.ID,
		AgentID:         agent.ID,
		SandboxID:       &sandbox.ID,
		Status:          "active",
		AgentTurnStatus: model.SessionAgentTurnActive,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", session.ID).Delete(&model.Session{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})

	sessionHandler := handler.NewSessionHandler(db).WithRuntimeStreamKey(encKey)
	router := chi.NewRouter()
	router.Get("/v1/sessions/{id}/stream", sessionHandler.Stream)
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgCopy := org
		r = middleware.WithOrg(r, &orgCopy)
		r = middleware.WithAPIKeyClaims(r, &middleware.APIKeyClaims{
			OrgID:  org.ID.String(),
			Scopes: []string{"sessions"},
		})
		router.ServeHTTP(w, r)
	}))
	t.Cleanup(apiServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		apiServer.URL+"/v1/sessions/"+session.ID.String()+"/stream?after_seq=7&ignored=do-not-forward", nil)
	if err != nil {
		t.Fatalf("create stream request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open API stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content type = %q", got)
	}
	if got := resp.Header.Get("X-Hivy-Stream-Id"); got != "runtime-stream-1" {
		t.Fatalf("stream id = %q", got)
	}
	if got := resp.Header.Get("X-Hivy-Stream-Next-Sequence"); got != "8" {
		t.Fatalf("next sequence = %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	firstFrame := readSSEFrame(t, reader)
	if !strings.Contains(firstFrame, "event: token") || !strings.Contains(firstFrame, `"text":"first"`) {
		t.Fatalf("first frame = %q", firstFrame)
	}
	select {
	case runtimeErr := <-runtimeErrors:
		t.Fatal(runtimeErr)
	default:
	}

	// The first frame was observable while the runtime handler was still blocked,
	// proving the API flushes live bytes instead of buffering the whole response.
	release()
	secondFrame := readSSEFrame(t, reader)
	if !strings.Contains(secondFrame, "event: turn_completed") {
		t.Fatalf("second frame = %q", secondFrame)
	}
}

func readSSEFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE frame: %v", err)
		}
		frame.WriteString(line)
		if line == "\n" || line == "\r\n" {
			return frame.String()
		}
	}
}
