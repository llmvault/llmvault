package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type assetListItem struct {
	ID          string `json:"id"`
	AgentID     string `json:"agent_id"`
	Path        string `json:"path"`
	Filename    string `json:"filename"`
	Key         string `json:"key"`
	PublicURL   string `json:"asset_url"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ListAssets handles GET /v1/assets.
//
// Returns agent drive assets owned by the caller's org, optionally filtered by:
//
//	?agent_id={uuid}      — files owned by one agent
//	?path={folder}           — exact match on the file's logical folder
//	?path_prefix={folder}    — folder tree match
//	?q={text}                — fuzzy match path, filename, content type, or storage key
//	?extension={ext}         — filename extension match
//	?content_type={prefix}   — content type prefix match
//	?created_from={time}     — created at lower bound
//	?created_to={time}       — created at upper bound
//	?sort_by={field}         — created_at, updated_at, filename, bytes, content_type, path
//	?sort_dir={asc|desc}     — default desc
//
// Filters combine. Ordered by created_at desc, cursor-paginated.
//
// @Summary List org assets
// @Description Lists agent drive assets owned by the caller's org. Optional filters: agent_id, path, path_prefix, q/search, extension, content_type, created_from, created_to. Ordered by created_at desc by default and cursor-paginated.
// @Tags assets
// @Produce json
// @Param agent_id query string false "Filter to files owned by this agent"
// @Param path query string false "Filter by exact folder label (empty = root)"
// @Param path_prefix query string false "Filter by folder tree prefix"
// @Param q query string false "Fuzzy search path, filename, content type, and storage key"
// @Param search query string false "Alias for q"
// @Param extension query string false "Filter by filename extension"
// @Param content_type query string false "Filter by content type prefix"
// @Param created_from query string false "Created-at lower bound (RFC3339 or YYYY-MM-DD)"
// @Param created_to query string false "Created-at upper bound (RFC3339 or YYYY-MM-DD)"
// @Param sort_by query string false "Sort field: created_at, updated_at, filename, bytes, content_type, path"
// @Param sort_dir query string false "Sort direction: asc or desc"
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Pagination cursor — unix-nanos from the previous page's tail for created_at/updated_at sorting"
// @Success 200 {object} paginatedResponse[assetListItem]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/assets [get]
func (h *UploadsHandler) ListAssets(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		if n > 200 {
			n = 200
		}
		limit = n
	}

	q := h.db.
		Table("agent_assets AS ea").
		Where("ea.org_id = ?", org.ID)

	if v := r.URL.Query().Get("agent_id"); v != "" {
		agentID, err := uuid.Parse(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
			return
		}
		q = q.Where("ea.agent_id = ?", agentID)
	}
	if v, ok := r.URL.Query()["path"]; ok && len(v) > 0 {
		q = q.Where("ea.path = ?", strings.Trim(strings.TrimSpace(v[0]), "/"))
	}
	if v := strings.Trim(strings.TrimSpace(r.URL.Query().Get("path_prefix")), "/"); v != "" {
		q = q.Where("ea.path = ? OR ea.path LIKE ?", v, escapeLike(v)+"/%")
	}
	if v := strings.TrimSpace(r.URL.Query().Get("q")); v == "" {
		if alias := strings.TrimSpace(r.URL.Query().Get("search")); alias != "" {
			v = alias
		}
		if v != "" {
			needle := "%" + escapeLike(strings.ToLower(v)) + "%"
			q = q.Where(
				"LOWER(ea.path) LIKE ? OR LOWER(ea.filename) LIKE ? OR LOWER(ea.content_type) LIKE ? OR LOWER(ea.key) LIKE ?",
				needle, needle, needle, needle,
			)
		}
	} else {
		needle := "%" + escapeLike(strings.ToLower(v)) + "%"
		q = q.Where(
			"LOWER(ea.path) LIKE ? OR LOWER(ea.filename) LIKE ? OR LOWER(ea.content_type) LIKE ? OR LOWER(ea.key) LIKE ?",
			needle, needle, needle, needle,
		)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("extension")); v != "" {
		ext := strings.TrimPrefix(strings.ToLower(v), ".")
		q = q.Where("LOWER(ea.filename) LIKE ?", "%."+escapeLike(ext))
	}
	if v := strings.TrimSpace(r.URL.Query().Get("content_type")); v != "" {
		q = q.Where("LOWER(ea.content_type) LIKE ?", escapeLike(strings.ToLower(v))+"%")
	}

	if v := strings.TrimSpace(r.URL.Query().Get("created_from")); v != "" {
		t, err := parseAssetListTime(v, false)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid created_from"})
			return
		}
		q = q.Where("ea.created_at >= ?", t)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("created_to")); v != "" {
		t, err := parseAssetListTime(v, true)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid created_to"})
			return
		}
		q = q.Where("ea.created_at <= ?", t)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("updated_from")); v != "" {
		t, err := parseAssetListTime(v, false)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid updated_from"})
			return
		}
		q = q.Where("ea.updated_at >= ?", t)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("updated_to")); v != "" {
		t, err := parseAssetListTime(v, true)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid updated_to"})
			return
		}
		q = q.Where("ea.updated_at <= ?", t)
	}

	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	sortColumn := "ea.created_at"
	switch sortBy {
	case "", "created_at":
		sortColumn = "ea.created_at"
	case "updated_at":
		sortColumn = "ea.updated_at"
	case "filename":
		sortColumn = "ea.filename"
	case "bytes":
		sortColumn = "ea.bytes"
	case "content_type":
		sortColumn = "ea.content_type"
	case "path":
		sortColumn = "ea.path"
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sort_by"})
		return
	}
	sortDir := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort_dir")))
	if sortDir == "" {
		sortDir = "desc"
	}
	if sortDir != "asc" && sortDir != "desc" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sort_dir"})
		return
	}

	if c := r.URL.Query().Get("cursor"); c != "" {
		if sortColumn != "ea.created_at" && sortColumn != "ea.updated_at" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cursor is only supported with created_at or updated_at sorting"})
			return
		}
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cursor"})
			return
		}
		op := "<"
		if sortDir == "asc" {
			op = ">"
		}
		q = q.Where(sortColumn+" "+op+" ?", time.Unix(0, n))
	}

	var rows []model.AgentAsset
	if err := q.Order(sortColumn + " " + sortDir).Limit(limit + 1).Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list assets"})
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	out := make([]assetListItem, len(rows))
	for i, r := range rows {
		out[i] = assetListItem{
			ID:          r.ID.String(),
			AgentID:     r.AgentID.String(),
			Path:        r.Path,
			Filename:    r.Filename,
			Key:         r.Key,
			PublicURL:   h.publicAssetURL(r.Key, r.PublicURL),
			ContentType: r.ContentType,
			Bytes:       r.Bytes,
			CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:   r.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}

	result := paginatedResponse[assetListItem]{Data: out, HasMore: hasMore}
	if hasMore {
		last := rows[len(rows)-1]
		cursorTime := last.CreatedAt
		if sortColumn == "ea.updated_at" {
			cursorTime = last.UpdatedAt
		}
		c := strconv.FormatInt(cursorTime.UnixNano(), 10)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

func parseAssetListTime(raw string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		return t.Add(24*time.Hour - time.Nanosecond), nil
	}
	return t, nil
}

func escapeLike(raw string) string {
	raw = strings.ReplaceAll(raw, `\`, `\\`)
	raw = strings.ReplaceAll(raw, `%`, `\%`)
	raw = strings.ReplaceAll(raw, `_`, `\_`)
	return raw
}
