package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type brandHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newBrandHarness(t *testing.T) *brandHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewBrandHandler(db)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFlexible(db))
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAPIKeyScopeOrJWT("brands"))
			r.Get("/orgs/current/brands", h.List)
			r.Get("/orgs/current/brands/{id}", h.Get)
			r.Get("/orgs/current/brands/{id}/assets", h.ListAssets)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdminOrAPIKey(db))
				r.Post("/orgs/current/brands", h.Create)
				r.Patch("/orgs/current/brands/{id}", h.Update)
				r.Delete("/orgs/current/brands/{id}", h.Archive)
				r.Post("/orgs/current/brands/{id}/default", h.SetDefault)
				r.Post("/orgs/current/brands/{id}/assets", h.CreateAsset)
				r.Delete("/orgs/current/brands/{id}/assets/{assetID}", h.DeleteAsset)
			})
		})
	})
	return &brandHarness{db: db, router: r}
}

func TestIntegration_BrandsCreatePatchAndAssetReferences(t *testing.T) {
	h := newBrandHarness(t)
	org := createTestOrg(t, h.db)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.BrandAsset{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Brand{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", owner.ID).Delete(&model.User{})
	})

	create := h.doJSON(t, http.MethodPost, "/v1/orgs/current/brands", org, owner, map[string]any{
		"name":       "Acme Product",
		"is_default": true,
		"colors": map[string]any{
			"version": 1,
			"tokens": []any{
				map[string]any{"id": "brand-blue", "value": "oklch(62% 0.18 251)", "roles": []string{"primary", "link"}},
			},
			"palettes": []any{
				map[string]any{"id": "core", "kind": "named", "colors": []any{
					map[string]any{"token": "brand-blue"},
					map[string]any{"value": "var(--accent)"},
				}},
			},
			"semantic": map[string]any{"primary": "brand-blue"},
		},
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create brand status=%d body=%s", create.Code, create.Body.String())
	}
	created := decodeBrandMutation(t, create)
	if !created.Brand.IsDefault || created.Brand.Slug != "acme-product" {
		t.Fatalf("unexpected created brand: %+v", created.Brand)
	}

	asset := h.doJSON(t, http.MethodPost, "/v1/orgs/current/brands/"+created.Brand.ID+"/assets", org, owner, map[string]any{
		"kind":         "logo",
		"role":         "primary",
		"name":         "Primary logo",
		"key":          "pub/o/" + org.ID.String() + "/brand-assets/logo.png",
		"public_url":   "https://cdn.example.test/logo.png",
		"content_type": "image/png",
		"bytes":        512,
		"width":        256,
		"height":       128,
	})
	if asset.Code != http.StatusCreated {
		t.Fatalf("create asset status=%d body=%s", asset.Code, asset.Body.String())
	}
	createdAsset := decodeBrandAssetMutation(t, asset)

	patch := h.doJSON(t, http.MethodPatch, "/v1/orgs/current/brands/"+created.Brand.ID, org, owner, map[string]any{
		"logos": map[string]any{
			"version":          1,
			"primary_asset_id": createdAsset.Asset.ID,
			"variants": []any{
				map[string]any{"id": "primary-light", "asset_id": createdAsset.Asset.ID, "role": "primary", "theme": "light"},
			},
		},
	})
	if patch.Code != http.StatusOK {
		t.Fatalf("patch logos status=%d body=%s", patch.Code, patch.Body.String())
	}

	del := h.doJSON(t, http.MethodDelete, "/v1/orgs/current/brands/"+created.Brand.ID+"/assets/"+createdAsset.Asset.ID, org, owner, nil)
	if del.Code != http.StatusConflict {
		t.Fatalf("delete referenced asset status=%d body=%s", del.Code, del.Body.String())
	}

	list := h.doJSON(t, http.MethodGet, "/v1/orgs/current/brands", org, owner, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != created.Brand.ID {
		t.Fatalf("unexpected brand list: %+v", page.Data)
	}
}

func TestIntegration_BrandsRejectInvalidColors(t *testing.T) {
	h := newBrandHarness(t)
	org := createTestOrg(t, h.db)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", owner.ID).Delete(&model.User{})
	})

	rr := h.doJSON(t, http.MethodPost, "/v1/orgs/current/brands", org, owner, map[string]any{
		"name": "Broken Colors",
		"colors": map[string]any{
			"tokens": []any{map[string]any{"id": "primary", "value": "not-a-color"}},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid colors status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_BrandMutationsRequireAdmin(t *testing.T) {
	h := newBrandHarness(t)
	org := createTestOrg(t, h.db)
	member := seedSessionUser(t, h.db, org.ID, "member")
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", member.ID).Delete(&model.User{})
	})

	rr := h.doJSON(t, http.MethodPost, "/v1/orgs/current/brands", org, member, map[string]any{"name": "Member Brand"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("member create status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_BrandRoutesRequireBrandsAPIKeyScope(t *testing.T) {
	h := newBrandHarness(t)
	org := createTestOrg(t, h.db)
	req := httptest.NewRequest(http.MethodGet, "/v1/orgs/current/brands", nil)
	req = middleware.WithOrg(req, &org)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  uuid.NewString(),
		OrgID:  org.ID.String(),
		Scopes: []string{"sessions"},
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("wrong scope status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func (h *brandHarness) doJSON(t *testing.T, method, path string, org model.Org, user model.User, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: user.ID.String(), OrgID: org.ID.String()})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func decodeBrandMutation(t *testing.T, rr *httptest.ResponseRecorder) struct {
	Brand struct {
		ID        string `json:"id"`
		Slug      string `json:"slug"`
		IsDefault bool   `json:"is_default"`
	} `json:"brand"`
} {
	t.Helper()
	var out struct {
		Brand struct {
			ID        string `json:"id"`
			Slug      string `json:"slug"`
			IsDefault bool   `json:"is_default"`
		} `json:"brand"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode brand: %v\n%s", err, rr.Body.String())
	}
	return out
}

func decodeBrandAssetMutation(t *testing.T, rr *httptest.ResponseRecorder) struct {
	Asset struct {
		ID string `json:"id"`
	} `json:"asset"`
} {
	t.Helper()
	var out struct {
		Asset struct {
			ID string `json:"id"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode brand asset: %v\n%s", err, rr.Body.String())
	}
	return out
}
