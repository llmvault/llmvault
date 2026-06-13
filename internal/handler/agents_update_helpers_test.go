package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *agentHarness) patchAgent(t *testing.T, m orgWithMember, agentID uuid.UUID, body any, role string) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest(http.MethodPatch, "/v1/agents/"+agentID.String(), buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *agentHarness) deleteAgent(t *testing.T, m orgWithMember, agentID uuid.UUID, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/v1/agents/"+agentID.String(), nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *agentHarness) patchAgentModel(t *testing.T, m orgWithMember, agentID uuid.UUID, body any, role string) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest(http.MethodPatch, "/v1/agents/"+agentID.String()+"/model", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *agentHarness) rebootAgentSandbox(t *testing.T, m orgWithMember, agentID uuid.UUID, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agentID.String()+"/sandbox/reboot", nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *agentHarness) getAgentAvailableConnections(t *testing.T, m orgWithMember, agentID uuid.UUID, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/agents/"+agentID.String()+"/connections/available", nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *agentHarness) seedAgentConnection(t *testing.T, m orgWithMember, provider string) model.Connection {
	t.Helper()
	integ := createTestIntegration(t, h.db, provider)
	conn := model.Connection{
		OrgID:             m.org.ID,
		UserID:            m.user.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: provider + "-" + uuid.NewString()[:8],
		Meta:              model.JSON{},
	}
	if err := h.db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", conn.ID).Delete(&model.Connection{}) })
	return conn
}
