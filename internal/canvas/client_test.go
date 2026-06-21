package canvas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
)

func TestClientUsesPenpotHivyControlPlaneContract(t *testing.T) {
	key := "test-canvas-control-plane-key"
	teamID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	profileID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	projectID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	fileID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "Bearer "+key {
			t.Fatalf("authorization header = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		switch r.URL.Path {
		case "/api/hivy/teams":
			if body["team-id"] != teamID.String() || body["hivy-id"] != "org-1" {
				t.Fatalf("team payload = %#v", body)
			}
			writeTestJSON(w, map[string]string{"team-id": teamID.String(), "hivy-id": "org-1", "default-project-id": projectID.String()})
		case "/api/hivy/profiles":
			if body["profile-id"] != profileID.String() || body["fullname"] != "Ada" {
				t.Fatalf("profile payload = %#v", body)
			}
			writeTestJSON(w, map[string]string{"profile-id": profileID.String(), "team-id": teamID.String(), "hivy-id": "user-1-org-1", "mcp-token": "mtok", "mcp-url": "https://canvas.test/mcp/stream?userToken=mtok"})
		case "/api/hivy/projects":
			if body["project-id"] != projectID.String() || body["team-id"] != teamID.String() {
				t.Fatalf("project payload = %#v", body)
			}
			writeTestJSON(w, map[string]string{"project-id": projectID.String(), "team-id": teamID.String()})
		case "/api/hivy/files":
			if body["file-id"] != fileID.String() || body["project-id"] != projectID.String() || body["profile-id"] != profileID.String() {
				t.Fatalf("file payload = %#v", body)
			}
			writeTestJSON(w, map[string]string{"file-id": fileID.String(), "project-id": projectID.String(), "team-id": teamID.String()})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		CanvasPublicURL:       "https://canvas.test",
		CanvasAPIBaseURL:      server.URL,
		CanvasControlPlaneKey: key,
	})
	if _, err := client.UpsertTeam(context.Background(), TeamInput{TeamID: teamID, HivyID: "org-1", Name: "Org"}); err != nil {
		t.Fatalf("UpsertTeam: %v", err)
	}
	if profile, err := client.UpsertProfile(context.Background(), ProfileInput{ProfileID: profileID, TeamID: teamID, HivyID: "user-1-org-1", Email: "ada@example.test", Fullname: "Ada"}); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	} else if profile.MCPURL == "" {
		t.Fatalf("expected mcp url")
	}
	if _, err := client.UpsertProject(context.Background(), ProjectInput{ProjectID: projectID, TeamID: teamID, Name: "Project"}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	if _, err := client.UpsertFile(context.Background(), FileInput{FileID: fileID, ProjectID: projectID, ProfileID: &profileID, Name: "File"}); err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}
	want := []string{"/api/hivy/teams", "/api/hivy/profiles", "/api/hivy/projects", "/api/hivy/files"}
	if len(seen) != len(want) {
		t.Fatalf("paths = %#v, want %#v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("paths = %#v, want %#v", seen, want)
		}
	}
}

func TestMintSessionJWTUsesOneYearTTLAndCanvasClaims(t *testing.T) {
	key := "test-canvas-control-plane-key"
	profileID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	teamID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fileID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	pageID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	client := NewClient(&config.Config{
		CanvasPublicURL:       "https://canvas.test",
		CanvasAPIBaseURL:      "https://canvas-internal.test",
		CanvasControlPlaneKey: key,
	})
	tokenString, err := client.MintSessionJWT(profileID, teamID, &fileID, &pageID)
	if err != nil {
		t.Fatalf("MintSessionJWT: %v", err)
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("parse token: valid=%v err=%v", token.Valid, err)
	}
	if claims["iss"] != "hivy" || claims["aud"] != "penpot-canvas" {
		t.Fatalf("claims = %#v", claims)
	}
	if claims["profile_id"] != profileID.String() || claims["team_id"] != teamID.String() || claims["file_id"] != fileID.String() || claims["page_id"] != pageID.String() {
		t.Fatalf("claims = %#v", claims)
	}
	exp, err := claims.GetExpirationTime()
	if err != nil {
		t.Fatalf("expiration: %v", err)
	}
	ttl := time.Until(exp.Time)
	if ttl < SessionTTL-time.Minute || ttl > SessionTTL+time.Minute {
		t.Fatalf("ttl = %s, want about %s", ttl, SessionTTL)
	}
}

func writeTestJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
