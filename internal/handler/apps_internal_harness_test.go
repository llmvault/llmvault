package handler_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// appsHarness exercises the internal app API (apps plan §1.2): one app bound
// to one sheet, plus a second sheet in the SAME org and channel so tests can
// prove the app cannot reach beyond its bound sheet.
type appsHarness struct {
	db      *gorm.DB
	svc     *sheets.Service
	router  *chi.Mux
	handler *handler.AppsInternalHandler

	org     model.Org
	user    model.User
	agent   model.Agent
	channel model.Channel

	sheet  *sheets.SheetStructure
	page   sheets.PageStructure
	fields map[string]string // field name -> id

	unboundSheet *sheets.SheetStructure // same org+channel, NOT bound to the app
	unboundPage  sheets.PageStructure

	app    model.App
	secret string

	// actorKey signs launch-style X-Hivy-App-Actor tokens; its public half is
	// wired into the handler as the actor verifier.
	actorKey *rsa.PrivateKey
}

func newAppsHarness(t *testing.T) *appsHarness {
	t.Helper()
	db := connectTestDB(t)
	h := &appsHarness{db: db, svc: sheets.NewService(db)}
	h.org = createTestOrg(t, db)
	h.user = model.User{ID: uuid.New(), Email: "apps-" + uuid.NewString() + "@example.com", Name: "Apps User"}
	if err := db.Create(&h.user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	membership := model.OrgMembership{UserID: h.user.ID, OrgID: h.org.ID, Role: "member"}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create org membership: %v", err)
	}
	// The channel's default agent carries no cleanup of its own so the org
	// cascade removes it with the channel (fk_channels_default_agent is
	// RESTRICT). The app row cascades with the org too.
	h.agent = model.Agent{ID: uuid.New(), OrgID: &h.org.ID, Name: "Apps Agent " + uuid.NewString(), Model: "test", Status: "active"}
	h.channel = model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "apps-ch-" + uuid.NewString(), DefaultAgentID: h.agent.ID}
	for _, seed := range []any{&h.agent, &h.channel} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("create channel seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Delete(&model.App{}, "org_id = ?", h.org.ID)
		db.Delete(&model.Sheet{}, "org_id = ?", h.org.ID)
		db.Delete(&model.OrgMembership{}, "user_id = ?", h.user.ID)
		db.Delete(&model.User{}, "id = ?", h.user.ID)
	})

	ctx := context.Background()
	sheet, err := h.svc.CreateSheet(ctx, h.org.ID, sheets.CreateSheetRequest{
		Name: "Tasks",
		Pages: []sheets.PageSpec{{Name: "Tasks", Fields: []sheets.FieldSpec{
			{Name: "Title", Type: sheets.FieldTypeText},
			{Name: "Score", Type: sheets.FieldTypeNumber},
		}}},
	}, sheets.Actor{UserID: &h.user.ID, ChannelID: h.channel.ID})
	if err != nil {
		t.Fatalf("create sheet: %v", err)
	}
	h.sheet = sheet
	h.page = sheet.Pages[0]
	h.fields = map[string]string{}
	for _, field := range h.page.Fields {
		h.fields[field.Name] = field.ID
	}

	unbound, err := h.svc.CreateSheet(ctx, h.org.ID, sheets.CreateSheetRequest{
		Name: "Unbound",
		Pages: []sheets.PageSpec{{Name: "Secrets", Fields: []sheets.FieldSpec{
			{Name: "Secret", Type: sheets.FieldTypeText},
		}}},
	}, sheets.Actor{UserID: &h.user.ID, ChannelID: h.channel.ID})
	if err != nil {
		t.Fatalf("create unbound sheet: %v", err)
	}
	h.unboundSheet = unbound
	h.unboundPage = unbound.Pages[0]

	// The app secret is stored encrypted only (SandboxEncKey pattern).
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("generate enc key: %v", err)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("build enc key: %v", err)
	}
	secret, err := model.GenerateAppSecret()
	if err != nil {
		t.Fatalf("generate app secret: %v", err)
	}
	encrypted, err := encKey.EncryptString(secret)
	if err != nil {
		t.Fatalf("encrypt app secret: %v", err)
	}
	h.secret = secret
	h.app = model.App{
		ID: uuid.New(), OrgID: h.org.ID, ChannelID: h.channel.ID, SheetID: h.sheet.Sheet.ID,
		Slug: "tasks-app-" + uuid.NewString()[:8], Name: "Tasks App",
		EncryptedAppSecret: encrypted, CreatedByAgentID: &h.agent.ID, CreatedByUserID: &h.user.ID,
	}
	if err := db.Create(&h.app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}

	actorKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate actor key: %v", err)
	}
	h.actorKey = actorKey

	appsHandler := handler.NewAppsInternalHandler(db, h.svc, encKey).
		WithPresigner(fakeSheetsPresigner{}).
		WithActorVerifier(&actorKey.PublicKey)
	h.handler = appsHandler
	h.router = newAppsInternalRouter(appsHandler)
	return h
}

// actorToken mints a platform-signed launch-style JWT attributing an action to
// userID for this harness's app, matching what apps_internal.verifyAppActorToken
// accepts. appIDOverride, when non-empty, forges a token bound to a different app.
func (h *appsHarness) actorToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	return h.actorTokenForApp(t, userID, h.app.ID.String())
}

func (h *appsHarness) actorTokenForApp(t *testing.T, userID uuid.UUID, appID string) string {
	t.Helper()
	now := time.Now()
	claims := jwt.MapClaims{
		"app_id":  appID,
		"org_id":  h.org.ID.String(),
		"user_id": userID.String(),
		"iss":     "hivy",
		"aud":     "hivy-app",
		"sub":     userID.String(),
		"iat":     now.Unix(),
		"exp":     now.Add(time.Minute).Unix(),
		"jti":     uuid.NewString(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(h.actorKey)
	if err != nil {
		t.Fatalf("sign actor token: %v", err)
	}
	return signed
}

// newAppsInternalRouter mirrors production registration (mountInternalAppRoutes).
func newAppsInternalRouter(appsHandler *handler.AppsInternalHandler) *chi.Mux {
	router := chi.NewRouter()
	router.Route("/internal/apps/{appID}/v1", func(r chi.Router) {
		r.Get("/sheet", appsHandler.SheetStructure)
		r.Get("/live", appsHandler.Live)
		r.Route("/pages/{pageID}", func(r chi.Router) {
			r.Post("/rows/query", appsHandler.QueryRows)
			r.Post("/rows", appsHandler.InsertRows)
			r.Patch("/rows", appsHandler.UpdateRows)
			r.Delete("/rows", appsHandler.DeleteRows)
			r.Post("/attachments/download-url", appsHandler.AttachmentDownloadURL)
		})
	})
	return router
}

// do issues a request with the given bearer and optional X-Hivy-App-Actor.
func (h *appsHarness) do(t *testing.T, method, path, bearer, actor string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if actor != "" {
		req.Header.Set("X-Hivy-App-Actor", actor)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *appsHarness) appPath(suffix string) string {
	return "/internal/apps/" + h.app.ID.String() + "/v1" + suffix
}

func (h *appsHarness) pagePath(pageID uuid.UUID, suffix string) string {
	return h.appPath("/pages/" + pageID.String() + suffix)
}

// lastOperation returns the newest sheet_operations row for a page, used to
// assert actor attribution.
func (h *appsHarness) lastOperation(t *testing.T, pageID uuid.UUID) model.SheetOperation {
	t.Helper()
	var op model.SheetOperation
	if err := h.db.Where("page_id = ?", pageID).Order("created_at DESC").First(&op).Error; err != nil {
		t.Fatalf("load last operation: %v", err)
	}
	return op
}

// createOutsideUser seeds a user who is a member of orgID only (not the
// harness org), for non-member actor tests.
func createOutsideUser(t *testing.T, h *appsHarness, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	user := model.User{ID: uuid.New(), Email: "outsider-" + uuid.NewString() + "@example.com", Name: "Outsider"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create outside user: %v", err)
	}
	membership := model.OrgMembership{UserID: user.ID, OrgID: orgID, Role: "member"}
	if err := h.db.Create(&membership).Error; err != nil {
		t.Fatalf("create outside membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Delete(&model.OrgMembership{}, "user_id = ?", user.ID)
		h.db.Delete(&model.User{}, "id = ?", user.ID)
	})
	return user.ID
}

func decodeAppRows(t *testing.T, body []byte) []struct {
	ID   string         `json:"id"`
	Data map[string]any `json:"data"`
} {
	t.Helper()
	var decoded struct {
		Rows []struct {
			ID   string         `json:"id"`
			Data map[string]any `json:"data"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode rows response: %v", err)
	}
	return decoded.Rows
}
