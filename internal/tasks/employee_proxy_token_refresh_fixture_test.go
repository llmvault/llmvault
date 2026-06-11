package tasks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type proxyRefreshRuntime struct {
	mu        sync.Mutex
	envCalls  int
	lastEnv   map[string]string
	envStatus int
}

type employeeProxyTokenRefreshFixture struct {
	db          *gorm.DB
	server      *httptest.Server
	runtime     *proxyRefreshRuntime
	enqueuer    *enqueue.MockClient
	handler     *EmployeeProxyTokenRefreshHandler
	compileDeps employeeruntime.CompileDeps
	org         model.Org
	agent       model.Employee
	sandbox     model.Sandbox
}

func newEmployeeProxyTokenRefreshFixture(t *testing.T, envStatus int) *employeeProxyTokenRefreshFixture {
	t.Helper()
	db := openTasksMemoryTestDB(t)
	encKey := testTasksEncKey(t)
	cfg := &config.Config{ProxyHost: "proxy.hivy.test"}
	runtime := &proxyRefreshRuntime{envStatus: envStatus}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/readyz":
			w.WriteHeader(http.StatusOK)
		case "/config":
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"agent":{"name":"Hivy"},"system_prompt":{"cacheable_segments":[{"type":"static_text","config":{"content":"test"}}],"dynamic_segments":[]},"model":{"provider":"openai_compatible","base_url":"https://proxy.test/v1","model_id":"test","api_key_env":"HIVY_PROXY_API_KEY","temperature":0,"max_output_tokens":1000},"tools":[]}`))
				return
			}
			var req employeeruntime.ConfigUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			runtime.mu.Lock()
			runtime.envCalls++
			runtime.lastEnv = req.RuntimeEnv
			status := runtime.envStatus
			runtime.mu.Unlock()
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"env_key_count": len(req.RuntimeEnv)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	org := model.Org{Name: "proxy-refresh-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	cred := model.Credential{
		OrgID:        &org.ID,
		Label:        "proxy-refresh",
		BaseURL:      "https://proxy.test",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	agent := model.Employee{
		OrgID:         &org.ID,
		Name:          "employee-" + uuid.NewString()[:8],
		IsEmployee:    true,
		Harness:       "employee-sandbox",
		Model:         employeeruntime.DefaultEmployeeModel,
		CredentialID:  &cred.ID,
		Status:        "active",
		SystemPrompt:  "test employee",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		Integrations:  model.JSON{},
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	runtimeSecret := "runtime-secret-" + uuid.NewString()
	encryptedKey, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		EmployeeID:             &agent.ID,
		ExternalID:             "proxy-refresh-external",
		RuntimeURL:             server.URL,
		EncryptedRuntimeSecret: encryptedKey,
		Status:                 string(sandbox.StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	enqueuer := &enqueue.MockClient{}
	provider := &employeeUpgradeProvider{endpoint: server.URL}
	orch := sandbox.NewOrchestrator(db, provider, encKey, cfg)
	compileDeps := employeeruntime.CompileDeps{
		DB:         db,
		EncKey:     encKey,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}
	handler := NewEmployeeProxyTokenRefreshHandler(db, orch, compileDeps, enqueuer)
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		db.Where("org_id = ?", org.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.Employee{})
		db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return &employeeProxyTokenRefreshFixture{
		db: db, server: server, runtime: runtime, enqueuer: enqueuer,
		handler: handler, compileDeps: compileDeps, org: org, agent: agent, sandbox: sb,
	}
}

func requireProxyRefreshTask(t *testing.T, enqueuer *enqueue.MockClient) enqueue.EnqueuedTask {
	t.Helper()
	for _, task := range enqueuer.Tasks() {
		if task.TypeName == TypeEmployeeProxyTokenRefresh {
			return task
		}
	}
	t.Fatalf("expected %s task to be enqueued", TypeEmployeeProxyTokenRefresh)
	return enqueue.EnqueuedTask{}
}
