package hindsight

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestValidateRetainTagsProviderScope(t *testing.T) {
	db, orgID, agent := newMemoryTagTestDB(t)

	validated, err := ValidateRetainTags(context.Background(), db, agent, MemoryTagInput{
		Scope:      "provider",
		Provider:   "github-app",
		MemoryType: "technical_context",
	})
	if err != nil {
		t.Fatalf("validate tags: %v", err)
	}
	want := []string{"scope:provider", "provider:github-app", "memory_type:technical_context", "source:manual"}
	for _, tag := range want {
		if !hasString(validated.RetainTags, tag) {
			t.Fatalf("missing tag %q in %#v for org %s", tag, validated.RetainTags, orgID)
		}
	}
	for _, tag := range validated.RetainTags {
		if strings.HasPrefix(tag, "org:") || strings.HasPrefix(tag, "agent:") {
			t.Fatalf("unexpected scoped tag %q in %#v", tag, validated.RetainTags)
		}
	}
}

func TestValidateRetainTagsGitHubRepositoryScope(t *testing.T) {
	db, _, agent := newMemoryTagTestDB(t)

	validated, err := ValidateRetainTags(context.Background(), db, agent, MemoryTagInput{
		Scope:        "resource",
		Provider:     "github-app",
		ResourceType: "repository",
		ResourceID:   "UseHivy/UseHivy.com",
		MemoryType:   "technical_context",
	})
	if err != nil {
		t.Fatalf("validate tags: %v", err)
	}
	want := "resource:github-app:repository:usehivy/usehivy.com"
	if !hasString(validated.RetainTags, want) {
		t.Fatalf("missing resource tag %q in %#v", want, validated.RetainTags)
	}
}

func TestValidateRecallTagsRejectsInvalidResourceWithAllowedOptions(t *testing.T) {
	db, _, agent := newMemoryTagTestDB(t)

	_, err := ValidateRecallTags(context.Background(), db, agent, MemoryTagInput{
		Scope:        "resource",
		Provider:     "github-app",
		ResourceType: "repository",
		ResourceID:   "missing/repo",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	text := err.Error()
	for _, want := range []string{"resource_id", "missing/repo", "github-app", "repository", "usehivy/usehivy.com"} {
		if !strings.Contains(text, want) {
			t.Fatalf("validation error %q missing %q", text, want)
		}
	}
}

func newMemoryTagTestDB(t *testing.T) (*gorm.DB, uuid.UUID, *model.Agent) {
	t.Helper()
	db := openHindsightBankTestDB(t)
	orgID := uuid.New()
	agentID := uuid.New()
	userID := uuid.New()
	integrationID := uuid.New()
	connectionID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "memory-tags-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if err := db.Create(&model.User{ID: userID, Email: "memory-tags-" + uuid.NewString()[:8] + "@example.com"}).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	integration := model.Integration{
		ID:          integrationID,
		UniqueKey:   "github-app-" + uuid.NewString()[:8],
		Provider:    "github-app",
		DisplayName: "GitHub",
	}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	conn := model.Connection{
		ID:                connectionID,
		OrgID:             orgID,
		UserID:            userID,
		IntegrationID:     integrationID,
		NangoConnectionID: "github-conn",
		Meta: model.JSON{"resources": map[string]any{
			"repository": []any{map[string]any{
				"id":        "usehivy/usehivy.com",
				"name":      "usehivy.com",
				"full_name": "usehivy/usehivy.com",
			}},
		}},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	description := ""
	agent := &model.Agent{
		ID:          agentID,
		OrgID:       &orgID,
		Name:        "memory-tags-agent-" + uuid.NewString()[:8],
		Description: &description,
		Model:       "test-model",
		Status:      "active",
		Resources:   model.JSON{},
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	return db, orgID, agent
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
