package precontext

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type fakeEnvVarLister struct {
	docs []EnvVarDoc
	err  error

	orgIDs     []uuid.UUID
	channelIDs []uuid.UUID
}

func (f *fakeEnvVarLister) ChannelEnvVars(_ context.Context, orgID, channelID uuid.UUID) ([]EnvVarDoc, error) {
	f.orgIDs = append(f.orgIDs, orgID)
	f.channelIDs = append(f.channelIDs, channelID)
	return f.docs, f.err
}

func envVarsRequest() Request {
	return Request{OrgID: uuid.New(), ChannelID: uuid.New()}
}

func fetchEnvVars(t *testing.T, lister EnvVarLister, req Request) string {
	t.Helper()
	service := NewService(Config{EnvVars: lister})
	out, err := service.fetchEnvVarsSection(context.Background(), req)
	if err != nil {
		t.Fatalf("fetchEnvVarsSection returned error: %v", err)
	}
	return out
}

func TestEnvVarsSectionRendersNamesAndDescriptions(t *testing.T) {
	lister := &fakeEnvVarLister{docs: []EnvVarDoc{
		{Name: "DATABASE_URL"},
		{Name: "STRIPE_API_KEY", Description: "Stripe secret key for the billing sandbox"},
	}}
	req := envVarsRequest()
	out := fetchEnvVars(t, lister, req)

	if !strings.HasPrefix(out, envVarsSectionTitle+"\n") {
		t.Fatalf("section title missing: %q", out)
	}
	if !strings.Contains(out, envVarsPreamble) {
		t.Fatalf("never-reveal preamble missing: %q", out)
	}
	if !strings.Contains(out, "- STRIPE_API_KEY — Stripe secret key for the billing sandbox") {
		t.Fatalf("described var line missing: %q", out)
	}
	if !strings.Contains(out, "- DATABASE_URL — (no description)") {
		t.Fatalf("undescribed var placeholder missing: %q", out)
	}
	if strings.Index(out, envVarsPreamble) > strings.Index(out, "- DATABASE_URL") {
		t.Fatalf("preamble must render before the variable lines: %q", out)
	}
	if len(lister.orgIDs) != 1 || lister.orgIDs[0] != req.OrgID ||
		len(lister.channelIDs) != 1 || lister.channelIDs[0] != req.ChannelID {
		t.Fatalf("lister scope orgs=%v channels=%v, want [%s]/[%s]", lister.orgIDs, lister.channelIDs, req.OrgID, req.ChannelID)
	}
}

func TestEnvVarsSectionOmittedWhenEmpty(t *testing.T) {
	if out := fetchEnvVars(t, &fakeEnvVarLister{}, envVarsRequest()); out != "" {
		t.Fatalf("expected empty section for no env vars, got %q", out)
	}
	// Blank names cannot carry the section on their own.
	lister := &fakeEnvVarLister{docs: []EnvVarDoc{{Name: "   ", Description: "orphan"}}}
	if out := fetchEnvVars(t, lister, envVarsRequest()); out != "" {
		t.Fatalf("expected empty section for blank-named vars, got %q", out)
	}
}

func TestEnvVarsSectionGuardsNilConfig(t *testing.T) {
	service := NewService(Config{})
	if out, err := service.fetchEnvVarsSection(context.Background(), envVarsRequest()); err != nil || out != "" {
		t.Fatalf("nil lister must be a silent no-op, got %q err %v", out, err)
	}
	lister := &fakeEnvVarLister{docs: []EnvVarDoc{{Name: "TOKEN_A"}}}
	for name, req := range map[string]Request{
		"missing org":     {ChannelID: uuid.New()},
		"missing channel": {OrgID: uuid.New()},
	} {
		if out := fetchEnvVars(t, lister, req); out != "" {
			t.Fatalf("%s must omit the section, got %q", name, out)
		}
	}
	if len(lister.orgIDs) != 0 {
		t.Fatalf("guarded requests must not hit the lister (%d calls)", len(lister.orgIDs))
	}
}

func TestBuildDegradesWhenEnvVarsFail(t *testing.T) {
	service := NewService(Config{EnvVars: &fakeEnvVarLister{err: errors.New("env vars down")}})
	service.sessions = func(context.Context, Request) (string, error) {
		return "## Recent sessions\n- session", nil
	}
	service.memories = func(context.Context, Request) (string, error) { return "", nil }

	req := envVarsRequest()
	req.AgentID = uuid.New()
	out, err := service.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 || strings.Contains(out[0], "Environment variables") {
		t.Fatalf("failed env vars source must only cost its own section: %#v", out)
	}
	if !strings.Contains(out[0], "Recent sessions") {
		t.Fatalf("other sections must survive an env vars failure: %#v", out)
	}
}

func TestBuildIncludesEnvVarsSection(t *testing.T) {
	service := NewService(Config{EnvVars: &fakeEnvVarLister{docs: []EnvVarDoc{
		{Name: "STRIPE_API_KEY", Description: "Stripe secret key"},
	}}})
	service.sessions = func(context.Context, Request) (string, error) {
		return "## Recent sessions\n- session", nil
	}
	service.memories = func(context.Context, Request) (string, error) { return "", nil }

	out, err := service.Build(context.Background(), envVarsRequest())
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 ||
		!strings.Contains(out[0], "## Environment variables") ||
		!strings.Contains(out[0], "- STRIPE_API_KEY — Stripe secret key") {
		t.Fatalf("env vars section missing from Build output: %#v", out)
	}
}

// TestEnvVarsSectionIsNeverTrimmed pins the no-trimming behavior: the section
// has no per-section or per-line byte budget, so every variable and its full
// description always render (only the shared TotalBudgetBytes cap in
// joinSections bounds the combined precontext).
func TestEnvVarsSectionIsNeverTrimmed(t *testing.T) {
	longDescription := strings.Repeat("d", 2048)
	docs := make([]EnvVarDoc, 0, 60)
	for i := 0; i < 60; i++ {
		docs = append(docs, EnvVarDoc{
			Name:        fmt.Sprintf("VAR_%02d", i),
			Description: longDescription,
		})
	}
	out := fetchEnvVars(t, &fakeEnvVarLister{docs: docs}, envVarsRequest())
	if !strings.Contains(out, envVarsPreamble) {
		t.Fatalf("never-reveal preamble missing: %q", out[:200])
	}
	for i := 0; i < 60; i++ {
		line := fmt.Sprintf("- VAR_%02d — %s", i, longDescription)
		if !strings.Contains(out, line) {
			t.Fatalf("var %02d was trimmed or dropped", i)
		}
	}
	if strings.Contains(out, "...") {
		t.Fatalf("section content was trimmed: %q", out[len(out)-80:])
	}
}

// TestEnvVarValuesAreUnrepresentable pins the defense-in-depth contract: the
// lister interface exchanges EnvVarDoc, which structurally cannot carry a
// value. Even a model.ChannelEnvVar with a populated encrypted value can only
// cross into precontext as name + description, so the rendered section can
// never contain a value.
func TestEnvVarValuesAreUnrepresentable(t *testing.T) {
	docType := reflect.TypeOf(EnvVarDoc{})
	if docType.NumField() != 2 {
		t.Fatalf("EnvVarDoc must carry exactly name + description, has %d fields", docType.NumField())
	}
	for _, name := range []string{"Name", "Description"} {
		field, ok := docType.FieldByName(name)
		if !ok || field.Type.Kind() != reflect.String {
			t.Fatalf("EnvVarDoc.%s must be a string field", name)
		}
	}

	const secret = "sk_live_SUPER_SECRET_VALUE_9f8e7d"
	row := model.ChannelEnvVar{
		Name:           "STRIPE_API_KEY",
		Description:    "Stripe secret key for the billing sandbox",
		EncryptedValue: []byte(secret),
	}
	// The only projection available to this package: name + description.
	lister := &fakeEnvVarLister{docs: []EnvVarDoc{{Name: row.Name, Description: row.Description}}}
	out := fetchEnvVars(t, lister, envVarsRequest())
	if strings.Contains(out, secret) || strings.Contains(out, "SUPER_SECRET") {
		t.Fatalf("rendered section leaked a value: %q", out)
	}
	if !strings.Contains(out, "- STRIPE_API_KEY — Stripe secret key for the billing sandbox") {
		t.Fatalf("name + description projection missing: %q", out)
	}
}
