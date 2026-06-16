package credentials_test

import (
	"context"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/registry"
)

func TestIntegration_ResolveForModelPrefersOrgCredential(t *testing.T) {
	db := connectTestDB(t)
	orgID := seedBYOKOrg(t, db)
	orgCred := seedBYOKCred(t, db, orgID, "openrouter")
	seedSystemCred(t, db, "openai", false)

	got, err := credentials.ResolveForModel(context.Background(), db, registry.Global(), orgID, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("ResolveForModel: %v", err)
	}
	if got.ID != orgCred.ID {
		t.Fatalf("resolved %s, want org credential %s", got.ID, orgCred.ID)
	}
}

func TestIntegration_ResolveForModelUsesSystemCredential(t *testing.T) {
	db := connectTestDB(t)
	orgID := seedBYOKOrg(t, db)
	sys := seedSystemCred(t, db, "openai", false)

	got, err := credentials.ResolveForModel(context.Background(), db, registry.Global(), orgID, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("ResolveForModel: %v", err)
	}
	if got.ID != sys.ID {
		t.Fatalf("resolved %s, want system credential %s", got.ID, sys.ID)
	}
}

func TestIntegration_ResolveForModelUsesFirstCreatedCredential(t *testing.T) {
	db := connectTestDB(t)
	orgID := seedBYOKOrg(t, db)
	first := seedBYOKCred(t, db, orgID, "openai")
	seedBYOKCred(t, db, orgID, "openai")

	got, err := credentials.ResolveForModel(context.Background(), db, registry.Global(), orgID, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("ResolveForModel: %v", err)
	}
	if got.ID != first.ID {
		t.Fatalf("resolved %s, want first credential %s", got.ID, first.ID)
	}
}

func TestIntegration_ResolveForModelRejectsUnknownModel(t *testing.T) {
	db := connectTestDB(t)
	orgID := seedBYOKOrg(t, db)
	_, err := credentials.ResolveForModel(context.Background(), db, registry.Global(), orgID, "not-a-model")
	if err == nil || !strings.Contains(err.Error(), "not in the catalog") {
		t.Fatalf("expected catalog error, got %v", err)
	}
}

func TestIntegration_ResolveForModelRequiresMatchingCredential(t *testing.T) {
	db := connectTestDB(t)
	orgID := seedBYOKOrg(t, db)
	seedSystemCred(t, db, "openai", false)

	_, err := credentials.ResolveForModel(context.Background(), db, registry.Global(), orgID, "claude-sonnet-4.6")
	if err == nil || !strings.Contains(err.Error(), "no active org or system credential supports model") {
		t.Fatalf("expected no matching credential error, got %v", err)
	}
}
