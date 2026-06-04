package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	dbi "github.com/usehivy/hivy/internal/databaseintegration"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type DatabaseProxyHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	kms    *crypto.KeyWrapper
}

func NewDatabaseProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, kms *crypto.KeyWrapper) *DatabaseProxyHandler {
	return &DatabaseProxyHandler{db: db, encKey: encKey, kms: kms}
}

func (h *DatabaseProxyHandler) Handle(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.handle(w, r, provider)
	}
}

func (h *DatabaseProxyHandler) handle(w http.ResponseWriter, r *http.Request, provider string) {
	ctx := r.Context()
	provider, err := dbi.NormalizeProvider(provider)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported database provider"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "database proxy only supports POST"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "employeeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee_id"})
		return
	}
	token := extractBearerToken(r)
	if token == "" || !h.authenticatedSandbox(ctx, agentID, token) {
		h.capture(ctx, provider, http.StatusUnauthorized, "invalid database proxy credentials", nil)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	employee, err := h.resolveEmployee(ctx, agentID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		h.capture(ctx, provider, status, "database proxy employee resolution failed", err)
		writeJSON(w, status, map[string]string{"error": "employee not found"})
		return
	}
	conn, err := h.resolveConnection(ctx, employee, provider)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		h.capture(ctx, provider, status, "database connection resolution failed", err)
		writeJSON(w, status, map[string]string{"error": "database connection not found"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	dsn, err := dbi.DecryptSecret(ctx, h.kms, conn.EncryptedDSN, conn.WrappedDEK)
	if err != nil {
		h.capture(ctx, provider, http.StatusInternalServerError, "database credential decrypt failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt database credentials"})
		return
	}
	result, err := executeDatabaseQuery(ctx, provider, dsn, body, dbi.PolicyFromJSON(conn.AccessPolicy))
	if err != nil {
		h.capture(ctx, provider, http.StatusBadGateway, "database proxy query failed", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *DatabaseProxyHandler) authenticatedSandbox(ctx context.Context, agentID uuid.UUID, bearerToken string) bool {
	var sandboxes []model.Sandbox
	if err := h.db.WithContext(ctx).Where("employee_id = ?", agentID).Find(&sandboxes).Error; err != nil {
		return false
	}
	for _, sb := range sandboxes {
		decryptedKey, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
		if err == nil && subtle.ConstantTimeCompare([]byte(bearerToken), []byte(decryptedKey)) == 1 {
			return true
		}
	}
	return false
}

func (h *DatabaseProxyHandler) resolveEmployee(ctx context.Context, agentID uuid.UUID) (model.Employee, error) {
	var employee model.Employee
	if err := h.db.WithContext(ctx).Where("id = ?", agentID).First(&employee).Error; err != nil {
		return model.Employee{}, err
	}
	if employee.OrgID == nil {
		return model.Employee{}, gorm.ErrRecordNotFound
	}
	return employee, nil
}

func (h *DatabaseProxyHandler) resolveConnection(ctx context.Context, employee model.Employee, provider string) (model.DatabaseConnection, error) {
	var conn model.DatabaseConnection
	return conn, h.db.WithContext(ctx).
		Where("org_id = ? AND provider = ? AND revoked_at IS NULL", *employee.OrgID, provider).
		Where("employee_id IS NULL OR employee_id = ?", employee.ID).
		Order("employee_id DESC, created_at ASC").
		First(&conn).Error
}

func (h *DatabaseProxyHandler) capture(ctx context.Context, provider string, status int, reason string, err error) {
	if err == nil {
		err = fmt.Errorf("%s", reason)
	}
	logging.FromContext(ctx).ErrorContext(ctx, reason, "provider", provider, "status", status, "error", err)
	logging.Capture(ctx, fmt.Errorf("database proxy %s %d: %w", provider, status, err))
}
