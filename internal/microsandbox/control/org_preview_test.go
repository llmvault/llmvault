package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestOrgPreviewPasswordCanBeStoredAndRead(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:org-preview-password?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.OrgPreviewSecret{}); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db, cfg: config.Config{APIToken: "api-token", PreviewPasswordKey: "password-key"}}

	putReq := httptest.NewRequest(http.MethodPut, "/v1/orgs/org_1/preview-password", strings.NewReader(`{"preview_password":"amber-linen-river"}`))
	putReq.Header.Set("Authorization", "Bearer api-token")
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	s.Routes().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body = %s", putRec.Code, putRec.Body.String())
	}

	var stored model.OrgPreviewSecret
	if err := db.First(&stored, "org_id = ?", "org_1").Error; err != nil {
		t.Fatal(err)
	}
	if string(stored.PasswordCiphertext) == "amber-linen-river" {
		t.Fatal("preview password was stored as plaintext")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/orgs/org_1/preview-password", nil)
	getReq.Header.Set("Authorization", "Bearer api-token")
	getRec := httptest.NewRecorder()
	s.Routes().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body = %s", getRec.Code, getRec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(getRec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["preview_password"] != "amber-linen-river" {
		t.Fatalf("preview_password = %q", out["preview_password"])
	}
}
