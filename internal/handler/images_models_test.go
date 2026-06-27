package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestImageGenerationModels_ReturnsCatalogForAvailableSystemProviders(t *testing.T) {
	db := newModelCatalogTestDB(t)
	seedModelCatalogCredential(t, db, "openrouter")
	seedModelCatalogCredential(t, db, "reve")
	h := handler.NewImageDescribeHandler(db, nil, registry.Global(), nil, "")

	rr := imageModels(t, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		DefaultRasterModel string `json:"default_raster_model"`
		DefaultVectorModel string `json:"default_vector_model"`
		Models             []struct {
			ID          string   `json:"id"`
			ProviderIDs []string `json:"provider_ids"`
			OpenWeights bool     `json:"open_weights"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.DefaultRasterModel != "reve-image" {
		t.Fatalf("default raster = %q", resp.DefaultRasterModel)
	}
	if resp.DefaultVectorModel != "recraft-v4.1-vector" {
		t.Fatalf("default vector = %q", resp.DefaultVectorModel)
	}

	foundFlux := false
	foundReve := false
	foundVector := false
	for _, model := range resp.Models {
		switch model.ID {
		case "reve-image":
			foundReve = true
			if len(model.ProviderIDs) != 1 || model.ProviderIDs[0] != "reve" {
				t.Fatalf("provider_ids for reve-image = %v", model.ProviderIDs)
			}
		case "flux.2-klein-4b":
			foundFlux = true
			if !model.OpenWeights {
				t.Fatal("flux.2-klein-4b should be marked open_weights")
			}
		case "recraft-v4.1-vector":
			foundVector = true
		}
		if model.ID != "reve-image" && (len(model.ProviderIDs) != 1 || model.ProviderIDs[0] != "openrouter") {
			t.Fatalf("provider_ids for %s = %v", model.ID, model.ProviderIDs)
		}
	}
	if !foundReve || !foundFlux || !foundVector {
		t.Fatalf("models missing reve=%v flux=%v vector=%v", foundReve, foundFlux, foundVector)
	}
}

func TestImageGenerationModels_MissingImageGenerationCredentialReturns503(t *testing.T) {
	db := newModelCatalogTestDB(t)
	h := handler.NewImageDescribeHandler(db, nil, registry.Global(), nil, "")

	rr := imageModels(t, h)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
}

func imageModels(t *testing.T, h *handler.ImageDescribeHandler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/images/models", nil)
	rr := httptest.NewRecorder()
	h.ListGenerationModels(rr, req)
	return rr
}

func newModelCatalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE credentials (
		id text PRIMARY KEY,
		org_id text,
		label text NOT NULL DEFAULT '',
		base_url text NOT NULL,
		auth_scheme text NOT NULL,
		encrypted_key blob NOT NULL,
		wrapped_dek blob NOT NULL,
		remaining integer,
		refill_amount integer,
		refill_interval text,
		last_refill_at datetime,
		provider_id text DEFAULT '',
		meta text DEFAULT '{}',
		revoked_at datetime,
		created_at datetime
	)`).Error; err != nil {
		t.Fatalf("create credentials table: %v", err)
	}
	return db
}

func seedModelCatalogCredential(t *testing.T, db *gorm.DB, providerID string) {
	t.Helper()
	cred := model.Credential{
		ID:           uuid.New(),
		Label:        "test-" + providerID,
		BaseURL:      "https://" + providerID + ".test/api/v1",
		AuthScheme:   "bearer",
		ProviderID:   providerID,
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
}
