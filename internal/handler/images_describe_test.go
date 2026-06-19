package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/storage"
	"github.com/usehivy/hivy/internal/system"
)

type fakeImageGateway struct {
	text string
	err  error
	call system.ForwardCall
}

func (g *fakeImageGateway) Complete(ctx context.Context, call system.ForwardCall) (*system.CompletionResult, error) {
	g.call = call
	if g.err != nil {
		return nil, g.err
	}
	return &system.CompletionResult{
		Text: g.text,
		Usage: system.Usage{
			InputTokens:  321,
			OutputTokens: 123,
		},
		Model: call.Request.Model,
	}, nil
}

func (g *fakeImageGateway) Stream(context.Context, system.ForwardCall, http.ResponseWriter) (*system.CompletionResult, error) {
	return nil, errors.New("not implemented")
}

type imageDescribeHarness struct {
	db      *gorm.DB
	router  *chi.Mux
	gateway *fakeImageGateway
	org     *model.Org
	user    *model.User
	asset   *model.AgentAsset
}

type fakeImageAssetReader struct {
	data []byte
	err  error
}

func (r fakeImageAssetReader) Open(context.Context, string) (io.ReadCloser, error) {
	if r.err != nil {
		return nil, r.err
	}
	return io.NopCloser(bytes.NewReader(r.data)), nil
}

func newImageDescribeHarness(t *testing.T, opts ...func(*imageDescribeHarnessConfig)) *imageDescribeHarness {
	t.Helper()
	cfg := imageDescribeHarnessConfig{
		contentType: "image/png",
		modelText:   validImageAnalysisJSON(),
		seedCred:    true,
		registry:    registry.Global(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	db := connectTestDB(t)
	kms := newSystemTaskKMS(t)
	upstreamBase := "https://openrouter.test/api/v1"
	var cred *model.Credential
	if cfg.seedCred {
		cred = seedSystemCredential(t, db, kms, upstreamBase, "openrouter")
	}

	org := &model.Org{
		ID:        uuid.New(),
		Name:      "img-desc-" + uuid.NewString()[:8],
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := &model.User{
		ID:               uuid.New(),
		Email:            "img-" + uuid.NewString()[:8] + "@test.local",
		Name:             "Image Tester",
		EmailConfirmedAt: tptr(time.Now()),
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	agent := &model.Agent{
		ID:     uuid.New(),
		OrgID:  &org.ID,
		Name:   "image-agent-" + uuid.NewString()[:8],
		Status: "active",
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := &model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		Status:                 "running",
		ExternalID:             "external",
		RuntimeURL:             "http://runtime.local",
		EncryptedRuntimeSecret: []byte("encrypted"),
	}
	if err := db.Create(sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	asset := &model.AgentAsset{
		ID:          uuid.New(),
		OrgID:       org.ID,
		AgentID:     agent.ID,
		SandboxID:   sandbox.ID,
		Path:        "uploads",
		Filename:    "screenshot.png",
		Key:         "pub/e/" + agent.ID.String() + "/uploads/screenshot.png",
		PublicURL:   "https://assets.test/screenshot.png",
		ContentType: cfg.contentType,
		Bytes:       100,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.Create(asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	gateway := &fakeImageGateway{text: cfg.modelText, err: cfg.gatewayErr}
	h := handler.NewImageDescribeHandler(db, kms, cfg.registry, gateway, "https://api.usehivy.test")
	if cfg.assetReader != nil {
		h.WithAssetReader(cfg.assetReader)
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req = middleware.WithOrg(req, org)
			req = middleware.WithUser(req, user)
			req = middleware.WithAuthClaims(req, &auth.AuthClaims{
				OrgID:  org.ID.String(),
				UserID: user.ID.String(),
			})
			next.ServeHTTP(w, req)
		})
	})
	r.Post("/v1/images/describe", h.Describe)

	out := &imageDescribeHarness{
		db:      db,
		router:  r,
		gateway: gateway,
		org:     org,
		user:    user,
		asset:   asset,
	}
	t.Cleanup(func() {
		if cred != nil {
			db.Where("id = ?", cred.ID).Delete(&model.Credential{})
		}
		db.Where("org_id = ?", org.ID).Delete(&model.Generation{})
		db.Where("id = ?", asset.ID).Delete(&model.AgentAsset{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return out
}

type imageDescribeHarnessConfig struct {
	contentType string
	modelText   string
	gatewayErr  error
	seedCred    bool
	registry    *registry.Registry
	assetReader storage.Reader
}

func withImageContentType(contentType string) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.contentType = contentType }
}

func withImageModelText(text string) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.modelText = text }
}

func withImageGatewayErr(err error) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.gatewayErr = err }
}

func withoutImageCredential() func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.seedCred = false }
}

func withImageRegistry(reg *registry.Registry) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.registry = reg }
}

func withImageAssetReader(reader storage.Reader) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.assetReader = reader }
}

func (h *imageDescribeHarness) describe(t *testing.T, assetID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"drive_asset_id":"` + assetID.String() + `","detail_level":"high"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/images/describe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestImageDescribe_OwnedImageReturnsStructuredAnalysis(t *testing.T) {
	h := newImageDescribeHarness(t)
	rr := h.describe(t, h.asset.ID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		DriveAssetID        string         `json:"drive_asset_id"`
		AssetURL            string         `json:"asset_url"`
		Filename            string         `json:"filename"`
		ContentType         string         `json:"content_type"`
		Category            string         `json:"category"`
		Confidence          float64        `json:"confidence"`
		Analysis            map[string]any `json:"analysis"`
		RenderedDescription string         `json:"rendered_description"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.DriveAssetID != h.asset.ID.String() || resp.Category != "product_ui" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.AssetURL == "" || !strings.Contains(resp.AssetURL, "/v1/assets/preview") {
		t.Fatalf("asset_url = %q", resp.AssetURL)
	}
	if !strings.Contains(resp.RenderedDescription, "Primary category: Product Ui") {
		t.Fatalf("rendered description missing category:\n%s", resp.RenderedDescription)
	}
	if strings.Contains(strings.ToLower(resp.RenderedDescription), "openrouter") ||
		strings.Contains(strings.ToLower(resp.RenderedDescription), "gemini-3.5-flash") {
		t.Fatalf("rendered description leaked backend model/provider:\n%s", resp.RenderedDescription)
	}
	if h.gateway.call.ProviderID != "openrouter" {
		t.Fatalf("provider = %q", h.gateway.call.ProviderID)
	}
	if h.gateway.call.Request.Model != "google/gemini-3.5-flash" {
		t.Fatalf("model = %q", h.gateway.call.Request.Model)
	}
	if h.gateway.call.Request.MaxTokens != 6000 {
		t.Fatalf("max tokens = %d", h.gateway.call.Request.MaxTokens)
	}
	userMsg := h.gateway.call.Request.Messages[1]
	if len(userMsg.Parts) != 2 || userMsg.Parts[1].Kind != system.LLMPartMedia {
		t.Fatalf("expected text+media message parts: %+v", userMsg.Parts)
	}
}

func TestImageDescribe_IncludesAutoExtractedMetadata(t *testing.T) {
	h := newImageDescribeHarness(t, withImageAssetReader(fakeImageAssetReader{data: transparentPNG(t)}))
	rr := h.describe(t, h.asset.ID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	userMsg := h.gateway.call.Request.Messages[1]
	if len(userMsg.Parts) == 0 || !strings.Contains(userMsg.Parts[0].Text, "Auto-extracted image metadata") {
		t.Fatalf("metadata missing from prompt: %+v", userMsg.Parts)
	}
	if !strings.Contains(userMsg.Parts[0].Text, "transparent_pixel_ratio") {
		t.Fatalf("transparency stats missing from prompt:\n%s", userMsg.Parts[0].Text)
	}

	var resp struct {
		Analysis            map[string]any `json:"analysis"`
		RenderedDescription string         `json:"rendered_description"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rawMetadata, ok := resp.Analysis["auto_extracted_image_metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing from analysis: %+v", resp.Analysis)
	}
	if rawMetadata["has_transparency"] != true {
		t.Fatalf("has_transparency = %#v metadata=%+v", rawMetadata["has_transparency"], rawMetadata)
	}
	if ratio, ok := rawMetadata["transparent_pixel_ratio"].(float64); !ok || ratio <= 0 {
		t.Fatalf("transparent_pixel_ratio = %#v metadata=%+v", rawMetadata["transparent_pixel_ratio"], rawMetadata)
	}
	if !strings.Contains(resp.RenderedDescription, "Auto-extracted image metadata") {
		t.Fatalf("rendered description missing metadata:\n%s", resp.RenderedDescription)
	}
}

func TestImageDescribe_MissingOrForeignAssetReturns404(t *testing.T) {
	h := newImageDescribeHarness(t)
	rr := h.describe(t, uuid.New())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestImageDescribe_NonImageReturns422(t *testing.T) {
	h := newImageDescribeHarness(t, withImageContentType("application/pdf"))
	rr := h.describe(t, h.asset.ID)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestImageDescribe_MissingOpenRouterCredentialReturns503(t *testing.T) {
	h := newImageDescribeHarness(t, withoutImageCredential())
	rr := h.describe(t, h.asset.ID)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertImageErrorCode(t, rr, "system_credential_unavailable")
}

func TestImageDescribe_MissingModelRouteReturns503(t *testing.T) {
	h := newImageDescribeHarness(t, withImageRegistry(&registry.Registry{}))
	rr := h.describe(t, h.asset.ID)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertImageErrorCode(t, rr, "system_model_unavailable")
}

func TestImageDescribe_UpstreamFailureReturns502(t *testing.T) {
	h := newImageDescribeHarness(t, withImageGatewayErr(&system.UpstreamError{StatusCode: 500, Body: "boom"}))
	rr := h.describe(t, h.asset.ID)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertImageErrorCode(t, rr, "upstream_error")
}

func TestImageDescribe_InvalidModelOutputReturns502(t *testing.T) {
	h := newImageDescribeHarness(t, withImageModelText("this is not json"))
	rr := h.describe(t, h.asset.ID)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertImageErrorCode(t, rr, "invalid_model_output")
}

func assertImageErrorCode(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	var er struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &er); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if er.ErrorCode != want {
		t.Fatalf("error_code = %q want %q body=%s", er.ErrorCode, want, rr.Body.String())
	}
}

func validImageAnalysisJSON() string {
	return `{
		"category": "product_ui",
		"confidence": 0.94,
		"summary": "A product UI screenshot of a settings page. It must not mention OpenRouter or gemini-3.5-flash in rendered backend context.",
		"visible_text": [
			{"text": "Settings", "location": "top-left", "confidence": 0.99, "role": "heading"},
			{"text": "Save changes", "location": "bottom-right", "confidence": 0.97, "role": "button"}
		],
		"layout": {
			"canvas": "desktop app frame with sidebar and main panel",
			"regions": [
				{"name": "Sidebar", "location": "left", "size": "about 25%", "contents": "navigation"},
				{"name": "Main panel", "location": "center-right", "size": "about 75%", "contents": "settings form"}
			],
			"hierarchy": "heading, form fields, primary action",
			"spacing_alignment": "aligned grid with moderate whitespace"
		},
		"objects": [
			{"name": "Save button", "type": "button", "location": "bottom-right", "attributes": ["primary", "enabled"]}
		],
		"colors": [
			{"name": "Background", "hex": "#F8FAFC", "usage": "background", "coverage": "dominant"},
			{"name": "Accent", "hex": "#2563EB", "usage": "accent", "coverage": "small"}
		],
		"states": [
			{"element": "Save button", "state": "active", "evidence": "blue filled button"}
		],
		"relationships": ["Sidebar controls navigation for the main settings panel"],
		"important_details": ["Primary action is Save changes"],
		"limitations": ["Small secondary labels may be unreadable"],
		"untrusted_image_instructions": [],
		"category_specific": {
			"screen_type": "settings",
			"product_context": "workspace configuration",
			"navigation": {"items": ["General", "Members"], "selected_item": "General"},
			"primary_workflow": "editing settings",
			"components": [{"name": "Save changes", "type": "button", "state": "enabled", "text": "Save changes", "location": "bottom-right"}],
			"forms": [],
			"data_displayed": [],
			"visual_design": {"density": "medium", "border_radius": "about 6px", "shadows": "subtle", "typography": "sans-serif", "icon_style": "outline"},
			"accessibility_notes": ["visible focus state is not clear"]
		}
	}`
}

func transparentPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}
	img.SetNRGBA(1, 1, color.NRGBA{R: 240, G: 30, B: 30, A: 255})
	img.SetNRGBA(2, 1, color.NRGBA{R: 240, G: 30, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode transparent png: %v", err)
	}
	return buf.Bytes()
}
