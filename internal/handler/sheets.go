package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// SheetsPresigner mints presigned GET URLs for attachment object keys.
type SheetsPresigner interface {
	PresignGet(ctx context.Context, key string) (string, error)
}

// SheetsHandler exposes the user-facing REST surface for sheets. Handlers are
// thin shells over internal/sheets service functions — the same functions the
// MCP tools use (locked architectural decision, plan §2b).
type SheetsHandler struct {
	db         *gorm.DB
	svc        *sheets.Service
	signingKey []byte
	presigner  SheetsPresigner
	redis      *redis.Client
}

func NewSheetsHandler(db *gorm.DB, svc *sheets.Service, signingKey []byte) *SheetsHandler {
	return &SheetsHandler{db: db, svc: svc, signingKey: signingKey}
}

// WithPresigner enables attachment download URLs.
func (h *SheetsHandler) WithPresigner(p SheetsPresigner) *SheetsHandler {
	h.presigner = p
	return h
}

// WithRedis enables the /live SSE endpoint.
func (h *SheetsHandler) WithRedis(client *redis.Client) *SheetsHandler {
	h.redis = client
	return h
}

type sheetFieldSpecRequest struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Options map[string]any `json:"options,omitempty"`
}

type sheetPageSpecRequest struct {
	Name   string                  `json:"name"`
	Fields []sheetFieldSpecRequest `json:"fields,omitempty"`
}

type createSheetRequest struct {
	ChannelID   string                 `json:"channel_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Icon        string                 `json:"icon,omitempty"`
	Slug        string                 `json:"slug,omitempty"`
	Pages       []sheetPageSpecRequest `json:"pages,omitempty"`
}

func fieldSpecsFrom(specs []sheetFieldSpecRequest) []sheets.FieldSpec {
	out := make([]sheets.FieldSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, sheets.FieldSpec{
			Name: spec.Name, Type: spec.Type, Options: model.JSON(spec.Options),
		})
	}
	return out
}

func (h *SheetsHandler) ListSheets(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	channelID, err := uuid.Parse(r.URL.Query().Get("channel_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel_id must be a uuid"})
		return
	}
	if !h.canUseSheetChannel(r.Context(), org.ID, channelID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	result, nextCursor, err := h.svc.ListSheets(r.Context(), org.ID, channelID, r.URL.Query().Get("search"), limit, r.URL.Query().Get("cursor"))
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	summaries := make([]sheetSummary, 0, len(result))
	for _, sheet := range result {
		summaries = append(summaries, sheetSummaryFrom(sheet))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sheets": summaries, "next_cursor": nextCursor})
}

func (h *SheetsHandler) CreateSheet(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	var req createSheetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name is required"})
		return
	}
	channelID, err := uuid.Parse(req.ChannelID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel_id must be a uuid"})
		return
	}
	if !h.canUseSheetChannel(r.Context(), org.ID, channelID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	create := sheets.CreateSheetRequest{
		Name: req.Name, Description: req.Description,
		Icon: req.Icon, Slug: req.Slug,
	}
	for _, page := range req.Pages {
		create.Pages = append(create.Pages, sheets.PageSpec{
			Name: page.Name, Fields: fieldSpecsFrom(page.Fields),
		})
	}
	actor := sheetsActor(r)
	actor.ChannelID = channelID
	structure, err := h.svc.CreateSheet(r.Context(), org.ID, create, actor)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, h.structureResponse(r, org.ID, structure))
}

func (h *SheetsHandler) GetSheet(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	sheetID, ok := sheetsPathUUID(w, r, "sheetID")
	if !ok {
		return
	}
	structure, err := h.svc.GetSheet(r.Context(), org.ID, sheetID)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.structureResponse(r, org.ID, structure))
}

type updateSheetRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}

func (h *SheetsHandler) UpdateSheet(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	sheetID, ok := sheetsPathUUID(w, r, "sheetID")
	if !ok {
		return
	}
	var req updateSheetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	sheet, err := h.svc.UpdateSheet(r.Context(), org.ID, sheetID, sheets.UpdateSheetRequest{
		Name: req.Name, Description: req.Description, Icon: req.Icon,
	})
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sheetSummaryFrom(*sheet))
}

func (h *SheetsHandler) ArchiveSheet(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	sheetID, ok := sheetsPathUUID(w, r, "sheetID")
	if !ok {
		return
	}
	if err := h.svc.ArchiveSheet(r.Context(), org.ID, sheetID); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

func (h *SheetsHandler) structureResponse(r *http.Request, orgID uuid.UUID, structure *sheets.SheetStructure) sheetStructureResponse {
	response := sheetStructureResponse{
		Sheet: sheetSummaryFrom(structure.Sheet),
		Pages: make([]sheetPageView, 0, len(structure.Pages)),
	}
	pageIDs := make([]uuid.UUID, 0, len(structure.Pages))
	for _, page := range structure.Pages {
		pageIDs = append(pageIDs, page.Page.ID)
	}
	counts, err := h.svc.PageRowCounts(r.Context(), orgID, pageIDs)
	if err != nil {
		counts = map[uuid.UUID]int64{}
	}
	for _, page := range structure.Pages {
		response.Pages = append(response.Pages, sheetPageViewFrom(page.Page, page.Fields, counts[page.Page.ID]))
	}
	return response
}
