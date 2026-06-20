package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc/pool"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

type wakeTestProvider struct {
	agentCreateProvider
	providerID string
	mu         sync.Mutex
	startCalls int
	startDelay time.Duration
}

func (p *wakeTestProvider) ID() string {
	if p.providerID != "" {
		return p.providerID
	}
	return ProviderMicrosandbox
}

func (p *wakeTestProvider) StartSandbox(context.Context, string) error {
	p.mu.Lock()
	p.startCalls++
	p.mu.Unlock()
	time.Sleep(p.startDelay)
	return nil
}

func (p *wakeTestProvider) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.startCalls
}

func TestWakeSandboxDedupesConcurrentProviderStarts(t *testing.T) {
	db := connectSandboxTestDB(t)
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("path = %s, want /healthz", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer health.Close()

	provider := &wakeTestProvider{
		agentCreateProvider: agentCreateProvider{endpoint: health.URL},
		providerID:          ProviderDaytona,
		startDelay:          40 * time.Millisecond,
	}
	orch := NewOrchestrator(db, provider, sandboxTestSymmetricKey(t), &config.Config{})
	sb := model.Sandbox{
		ID:                     uuid.New(),
		ProviderID:             ProviderDaytona,
		ExternalID:             "external-1",
		RuntimeURL:             health.URL,
		EncryptedRuntimeSecret: []byte("unused"),
		Status:                 string(StatusStopped),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatal(err)
	}

	p := pool.New().WithErrors().WithMaxGoroutines(8)
	for i := 0; i < 8; i++ {
		p.Go(func() error {
			local := sb
			_, err := orch.WakeSandbox(context.Background(), &local)
			if err != nil {
				return fmt.Errorf("wake sandbox: %w", err)
			}
			return nil
		})
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := provider.calls(); got != 1 {
		t.Fatalf("provider starts = %d, want 1", got)
	}

	var got model.Sandbox
	if err := db.First(&got, "id = ?", sb.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != string(StatusRunning) {
		t.Fatalf("status = %q, want running", got.Status)
	}
}

func TestWakeSandboxMicrosandboxIsInfraManagedNoop(t *testing.T) {
	db := connectSandboxTestDB(t)
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer health.Close()

	provider := &wakeTestProvider{
		agentCreateProvider: agentCreateProvider{endpoint: health.URL},
		providerID:          ProviderMicrosandbox,
	}
	orch := NewOrchestrator(db, provider, sandboxTestSymmetricKey(t), &config.Config{})
	sb := model.Sandbox{
		ID:                     uuid.New(),
		ProviderID:             ProviderMicrosandbox,
		ExternalID:             "external-1",
		RuntimeURL:             health.URL,
		EncryptedRuntimeSecret: []byte("unused"),
		Status:                 string(StatusStopped),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatal(err)
	}

	got, err := orch.WakeSandbox(context.Background(), &sb)
	if err != nil {
		t.Fatalf("wake sandbox: %v", err)
	}
	if provider.calls() != 0 {
		t.Fatalf("provider starts = %d, want 0", provider.calls())
	}
	if got.Status != string(StatusStopped) {
		t.Fatalf("status = %q, want stopped", got.Status)
	}

	var row model.Sandbox
	if err := db.First(&row, "id = ?", sb.ID).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != string(StatusStopped) {
		t.Fatalf("persisted status = %q, want stopped", row.Status)
	}
}
