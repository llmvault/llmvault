package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	dbi "github.com/usehivy/hivy/internal/databaseintegration"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestDatabaseProxyUsesTeamRedisConnectionGrant(t *testing.T) {
	db := connectTestDB(t)
	kms := newTestKMS(t)
	encKey := testSymmetricKey(t)
	redisAddr := testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := redisClient.Ping(t.Context()).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", redisAddr, err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })

	key := "handler:database-proxy:redis:user:1"
	if err := redisClient.Set(t.Context(), key, "Ada", 0).Err(); err != nil {
		t.Fatalf("seed redis key: %v", err)
	}
	t.Cleanup(func() { _ = redisClient.Del(t.Context(), key).Err() })

	proxy := handler.NewDatabaseProxyHandler(db, encKey, kms)
	org := createDatabaseScopeTestOrg(t, db)
	agent := createDatabaseScopeTestAgent(t, db, org.ID, "redis")
	secret := createDatabaseScopeTestSandbox(t, db, encKey, org.ID, agent.ID)
	connectionID := createRedisDatabaseScopeConnection(t, db, kms, org.ID, redisAddr)
	grantDatabaseConnectionToAgentTeam(t, db, org.ID, agent.ID, connectionID)

	r := chi.NewRouter()
	r.Post("/internal/database-proxy/redis/{agentID}", proxy.Handle("redis"))

	rr := postRedisDatabaseProxy(t, r, agent.ID, secret, key)
	if rr.Code != http.StatusOK {
		t.Fatalf("redis proxy got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Command string `json:"command"`
		Result  string `json:"result"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode redis response: %v", err)
	}
	if resp.Command != "GET" || resp.Result != "Ada" {
		t.Fatalf("redis response = %#v", resp)
	}
}

func createRedisDatabaseScopeConnection(t *testing.T, db *gorm.DB, kms *crypto.KeyWrapper, orgID uuid.UUID, dsn string) uuid.UUID {
	t.Helper()
	encryptedDSN, wrappedDEK, err := dbi.EncryptSecret(t.Context(), kms, dsn)
	if err != nil {
		t.Fatalf("encrypt Redis dsn: %v", err)
	}
	conn := model.DatabaseConnection{
		ID:             uuid.New(),
		OrgID:          orgID,
		Provider:       "redis",
		DisplayName:    "Redis",
		EncryptedDSN:   encryptedDSN,
		WrappedDEK:     wrappedDEK,
		SchemaSnapshot: model.RawJSON("{}"),
		AccessPolicy: dbi.PolicyToJSON(dbi.Policy{
			AllowedKeys: []string{"handler:database-proxy:redis:*"},
			MaxRows:     10,
		}),
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create Redis database connection: %v", err)
	}
	return conn.ID
}

func postRedisDatabaseProxy(t *testing.T, r http.Handler, agentID uuid.UUID, secret, key string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"command": "GET",
		"args":    []string{key},
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/database-proxy/redis/"+agentID.String(), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}
