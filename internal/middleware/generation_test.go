package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/observe"
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

			handler := Generation(gw, db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	return db
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
