package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/system"
)

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
	var row model.AgentAsset
	if err := h.db.Where("id = ?", h.asset.ID).First(&row).Error; err != nil {
		t.Fatalf("load asset: %v", err)
	}
	if row.Description == nil {
		t.Fatalf("expected persisted image description")
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(*row.Description), &stored); err != nil {
		t.Fatalf("decode stored description: %v", err)
	}
	if stored["category"] != resp.Category || stored["rendered_description"] != resp.RenderedDescription {
		t.Fatalf("stored description mismatch: %+v", stored)
	}
}

func TestImageDescribe_RuntimeSecretCanDescribeAgentAsset(t *testing.T) {
	h := newImageDescribeHarness(t)
	rr := h.runtimeDescribe(t, h.asset.ID, h.runtimeSecret)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		DriveAssetID        string `json:"drive_asset_id"`
		RenderedDescription string `json:"rendered_description"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.DriveAssetID != h.asset.ID.String() {
		t.Fatalf("drive_asset_id = %q, want %s", resp.DriveAssetID, h.asset.ID)
	}
	if !strings.Contains(resp.RenderedDescription, "Primary category: Product Ui") {
		t.Fatalf("rendered description missing category:\n%s", resp.RenderedDescription)
	}
}

func TestImageDescribe_UsesInlineAssetBytesWhenReaderAvailable(t *testing.T) {
	h := newImageDescribeHarness(t, withImageAssetReader(fakeImageAssetReader{data: transparentPNG(t)}))
	rr := h.runtimeDescribe(t, h.asset.ID, h.runtimeSecret)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	userMsg := h.gateway.call.Request.Messages[1]
	if len(userMsg.Parts) != 2 || userMsg.Parts[1].Kind != system.LLMPartMedia {
		t.Fatalf("expected text+media message parts: %+v", userMsg.Parts)
	}
	if got := userMsg.Parts[1].Text; !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("media input = %q, want inline data URL", got)
	}
	var resp struct {
		AssetURL string `json:"asset_url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.AssetURL, "/v1/assets/preview") {
		t.Fatalf("response asset_url = %q, want preview URL", resp.AssetURL)
	}
}

func TestImageDescribe_RuntimeSecretRejectsWrongBearer(t *testing.T) {
	h := newImageDescribeHarness(t)
	rr := h.runtimeDescribe(t, h.asset.ID, "wrong-secret")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
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
