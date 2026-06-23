package handler

import (
	"encoding/json"
	"net/http"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type BrandHandler struct {
	db *gorm.DB
}

func NewBrandHandler(db *gorm.DB) *BrandHandler {
	return &BrandHandler{db: db}
}

type createBrandRequest struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug,omitempty"`
	Description string          `json:"description,omitempty"`
	IsDefault   bool            `json:"is_default,omitempty"`
	Logos       json.RawMessage `json:"logos,omitempty"`
	Colors      json.RawMessage `json:"colors,omitempty"`
	Typography  json.RawMessage `json:"typography,omitempty"`
	Voice       json.RawMessage `json:"voice,omitempty"`
	Source      json.RawMessage `json:"source,omitempty"`
	RawImport   json.RawMessage `json:"raw_import,omitempty"`
}

type brandResponse struct {
	ID          string         `json:"id"`
	OrgID       string         `json:"org_id"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	Description string         `json:"description"`
	IsDefault   bool           `json:"is_default"`
	Logos       model.RawJSON  `json:"logos"`
	Colors      model.RawJSON  `json:"colors"`
	Typography  model.RawJSON  `json:"typography"`
	Voice       model.RawJSON  `json:"voice"`
	Source      model.RawJSON  `json:"source"`
	RawImport   *model.RawJSON `json:"raw_import,omitempty"`
	ArchivedAt  *string        `json:"archived_at,omitempty"`
	CreatedBy   *string        `json:"created_by,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

type brandMutationResponse struct {
	Brand brandResponse `json:"brand"`
}

// @Summary List brands
// @Description Returns active brands for the current organization.
// @Tags brands
// @Produce json
// @Param limit query int false "Maximum results to return"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[brandResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/brands [get]
func (h *BrandHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := orgForBrandRequest(w, r)
	if !ok {
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	q := h.db.WithContext(r.Context()).Where("org_id = ? AND archived_at IS NULL", org.ID)
	q = applyPagination(q, cursor, limit)

	var brands []model.Brand
	if err := q.Find(&brands).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list brands"})
		return
	}
	hasMore := len(brands) > limit
	if hasMore {
		brands = brands[:limit]
	}
	out := make([]brandResponse, len(brands))
	for i, brand := range brands {
		out[i] = brandToResponse(brand)
	}
	resp := paginatedResponse[brandResponse]{Data: out, HasMore: hasMore}
	if hasMore {
		last := brands[len(brands)-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary Create a brand
// @Description Creates a brand for the current organization. Admin-only.
// @Tags brands
// @Accept json
// @Produce json
// @Param body body createBrandRequest true "Brand parameters"
// @Success 201 {object} brandMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/brands [post]
func (h *BrandHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := orgForBrandRequest(w, r)
	if !ok {
		return
	}
	var req createBrandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	brand, err := h.brandFromCreateRequest(r, org.ID, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if brand.IsDefault {
			if err := tx.Model(&model.Brand{}).
				Where("org_id = ? AND archived_at IS NULL", org.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&brand).Error
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "brand already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create brand"})
		return
	}
	writeJSON(w, http.StatusCreated, brandMutationResponse{Brand: brandToResponse(brand)})
}

// @Summary Get a brand
// @Description Returns one active brand for the current organization.
// @Tags brands
// @Produce json
// @Param id path string true "Brand ID"
// @Success 200 {object} brandMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/brands/{id} [get]
func (h *BrandHandler) Get(w http.ResponseWriter, r *http.Request) {
	brand, ok := h.loadBrandForRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, brandMutationResponse{Brand: brandToResponse(brand)})
}

// @Summary Update a brand
// @Description Updates an active brand for the current organization. Admin-only.
// @Tags brands
// @Accept json
// @Produce json
// @Param id path string true "Brand ID"
// @Param body body object true "Fields to patch"
// @Success 200 {object} brandMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/brands/{id} [patch]
func (h *BrandHandler) Update(w http.ResponseWriter, r *http.Request) {
	brand, ok := h.loadBrandForRequest(w, r)
	if !ok {
		return
	}
	fields, ok := decodeBrandPatchFields(w, r)
	if !ok {
		return
	}
	updates, setDefault, err := h.brandUpdatesFromPatch(r, brand, fields)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "no fields to update"})
		return
	}
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if setDefault {
			if err := tx.Model(&model.Brand{}).
				Where("org_id = ? AND archived_at IS NULL", brand.OrgID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.Brand{}).
			Where("id = ? AND org_id = ? AND archived_at IS NULL", brand.ID, brand.OrgID).
			Updates(updates).Error
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "brand already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update brand"})
		return
	}
	h.writeReloadedBrand(w, r, brand.ID, brand.OrgID)
}
