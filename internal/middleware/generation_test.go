package middleware

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/observe"
	"github.com/usehivy/hivy/internal/registry"
)

func TestGenerationCapturesAgentProxyCallsAndPreservesBillingFlag(t *testing.T) {
	db := generationTestDB(t)
	orgID := uuid.New()
	credID := uuid.New()
	if err := db.Exec(`INSERT INTO credentials (id, provider_id) VALUES (?, ?)`, credID.String(), "openrouter").Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}

	for _, tc := range []struct {
		name     string
		isSystem bool
	}{
		{name: "system credential", isSystem: true},
		{name: "user credential", isSystem: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gw := &GenerationWriter{entries: make(chan model.Generation, 1)}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.RemoteAddr = "127.0.0.1:4321"
			req = WithClaims(req, &TokenClaims{
				OrgID:        orgID.String(),
				CredentialID: credID.String(),
				JTI:          "jti-" + tc.name,
				TokenType:    model.TokenTypeAgentProxy,
				IsSystem:     tc.isSystem,
			})

			handler := Generation(gw, db, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured, ok := observe.CapturedDataFromContext(r.Context())
				if !ok {
					t.Fatal("captured data missing from request context")
				}
				captured.Model = "gpt-4o-mini"
				captured.Usage = observe.UsageData{InputTokens: 10, OutputTokens: 5}
				captured.UpstreamStatus = http.StatusOK
			}))

			handler.ServeHTTP(httptest.NewRecorder(), req)

			select {
			case gen := <-gw.entries:
				if gen.ProviderID != "openrouter" {
					t.Fatalf("provider_id = %q, want openrouter", gen.ProviderID)
				}
				if gen.TokenJTI != "jti-"+tc.name {
					t.Fatalf("token_jti = %q", gen.TokenJTI)
				}
				if gen.IsSystem != tc.isSystem {
					t.Fatalf("is_system = %v, want %v", gen.IsSystem, tc.isSystem)
				}
			default:
				t.Fatal("agent_proxy call did not queue a generation")
			}
		})
	}
}

func generationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE credentials (id text PRIMARY KEY, provider_id text)`).Error; err != nil {
		t.Fatalf("create credentials table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE tokens (id text PRIMARY KEY, jti text, meta text)`).Error; err != nil {
		t.Fatalf("create tokens table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE sessions (id text PRIMARY KEY, sandbox_id text)`).Error; err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
	return db
}

func TestGenerationResolvesSessionFromTokenSandbox(t *testing.T) {
	db := generationTestDB(t)
	orgID := uuid.New()
	credID := uuid.New()
	sandboxID := uuid.New()
	sessionID := uuid.New()
	if err := db.Exec(`INSERT INTO credentials (id, provider_id) VALUES (?, ?)`, credID.String(), "openrouter").Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	if err := db.Exec(`INSERT INTO tokens (id, jti, meta) VALUES (?, ?, ?)`,
		uuid.NewString(), "jti-sandbox", `{"type":"agent_proxy","sandbox_id":"`+sandboxID.String()+`"}`).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := db.Exec(`INSERT INTO sessions (id, sandbox_id) VALUES (?, ?)`, sessionID.String(), sandboxID.String()).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	gw := &GenerationWriter{entries: make(chan model.Generation, 1)}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req = WithClaims(req, &TokenClaims{
		OrgID:        orgID.String(),
		CredentialID: credID.String(),
		JTI:          "jti-sandbox",
		TokenType:    model.TokenTypeAgentProxy,
		IsSystem:     true,
	})

	handler := Generation(gw, db, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, ok := observe.CapturedDataFromContext(r.Context())
		if !ok {
			t.Fatal("captured data missing from request context")
		}
		captured.Model = "gpt-4o-mini"
		captured.Usage = observe.UsageData{InputTokens: 10, OutputTokens: 5}
		captured.UpstreamStatus = http.StatusOK
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case gen := <-gw.entries:
		if gen.SessionID == nil || *gen.SessionID != sessionID {
			t.Fatalf("session_id = %v, want %s", gen.SessionID, sessionID)
		}
	default:
		t.Fatal("agent_proxy call did not queue a generation")
	}
}

func TestGenerationLeavesSessionNilWithoutSandboxMeta(t *testing.T) {
	db := generationTestDB(t)
	if err := db.Exec(`INSERT INTO tokens (id, jti, meta) VALUES (?, ?, ?)`,
		uuid.NewString(), "jti-plain", `{"type":"agent_proxy","user":"u1"}`).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	var gen model.Generation
	extractAttribution(db, nil, "jti-plain", &gen)
	if gen.SessionID != nil {
		t.Fatalf("session_id = %v, want nil", gen.SessionID)
	}
	if gen.UserID != "u1" {
		t.Fatalf("user_id = %q, want u1", gen.UserID)
	}
}

func TestBuildGenerationUsesFallbackProviderAndModel(t *testing.T) {
	db := generationTestDB(t)
	credentialID := uuid.New()
	captured := &observe.CapturedData{
		CredentialID: credentialID.String(),
		ProviderID:   "openrouter",
		Model:        "mimo-v2.5-pro",
		GenerationID: "openrouter-generation-id",
		Usage:        observe.UsageData{InputTokens: 10, OutputTokens: 5},
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	claims := &TokenClaims{
		OrgID:        uuid.NewString(),
		CredentialID: uuid.NewString(),
		TokenType:    model.TokenTypeAgentProxy,
		IsSystem:     true,
	}

	generation := buildGeneration(request, claims, captured, "xiaomi", registry.Global(), db, nil)

	if generation.CredentialID != credentialID {
		t.Fatalf("credential_id = %s, want fallback credential %s", generation.CredentialID, credentialID)
	}
	if generation.ProviderID != "openrouter" {
		t.Fatalf("provider_id = %q, want openrouter", generation.ProviderID)
	}
	if generation.Model != "mimo-v2.5-pro" {
		t.Fatalf("model = %q, want fallback model", generation.Model)
	}
	if generation.OpenRouterGenerationID == nil || *generation.OpenRouterGenerationID != captured.GenerationID {
		t.Fatalf("openrouter_generation_id = %v, want %q", generation.OpenRouterGenerationID, captured.GenerationID)
	}
}

func TestBuildGenerationUsesEngyProviderReportedCost(t *testing.T) {
	db := generationTestDB(t)
	captured := &observe.CapturedData{
		ProviderID: "engy",
		Model:      "engy-glm-5.2",
		Usage: observe.UsageData{
			InputTokens:     17,
			OutputTokens:    32,
			ProviderCostUSD: 0.000060,
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	claims := &TokenClaims{
		OrgID:        uuid.NewString(),
		CredentialID: uuid.NewString(),
		TokenType:    model.TokenTypeAgentProxy,
		IsSystem:     true,
	}

	generation := buildGeneration(request, claims, captured, "", registry.Global(), db, nil)

	if math.Abs(generation.Cost-0.000060) > 1e-12 {
		t.Fatalf("cost = %.12f, want Engy charge 0.000060", generation.Cost)
	}
	if generation.BillingCostSource != billing.CostSourceProvider {
		t.Fatalf("billing_cost_source = %q, want %q", generation.BillingCostSource, billing.CostSourceProvider)
	}
}

func TestTruncateValidUTF8SanitizesProviderErrorBytes(t *testing.T) {
	got := truncateValidUTF8("prefix \x8b\x00 suffix", 1000)
	if !utf8.ValidString(got) {
		t.Fatalf("error message is not valid UTF-8: %q", got)
	}
	if got != "prefix ?? suffix" {
		t.Fatalf("sanitized message = %q, want %q", got, "prefix ?? suffix")
	}
}

func TestTruncateValidUTF8DoesNotSplitMultibyteRune(t *testing.T) {
	got := truncateValidUTF8("abé", 3)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated message is not valid UTF-8: %q", got)
	}
	if got != "ab?" {
		t.Fatalf("truncated message = %q, want %q", got, "ab?")
	}
}
