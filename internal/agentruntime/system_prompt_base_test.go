package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestRenderBaseSystemPrompt_PopulatesIdentityTag(t *testing.T) {
	org := model.Org{Name: "ExampleCo", PromptCompany: "Builds field-service software."}

	agent := &model.Agent{Name: "Ari"}
	prompt := renderBaseSystemPrompt(context.Background(), nil, agent, org, true, "Coordinates engineering work.")

	for _, want := range []string{
		"<identity>",
		"You are Ari, a real teammate with one goal: get real team work done based on your responsibilities.",
		"Be proactive, take initiative, understand the business and your team, and execute your role in service of the company's goals.",
		"Your role is described this way: Coordinates engineering work.",
		"You work at ExampleCo.",
		"The company is described this way: Builds field-service software.",
		"</identity>",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("rendered base prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRenderBaseSystemPrompt_LeavesEmptyIdentityWhenOrgMissing(t *testing.T) {
	prompt := renderBaseSystemPrompt(context.Background(), nil, nil, model.Org{}, false, "")

	if strings.Contains(prompt, "You work at") || strings.Contains(prompt, "The company is described this way:") {
		t.Fatalf("rendered base prompt should not invent company identity:\n%s", prompt)
	}
	if !strings.Contains(prompt, "You are Hivy, a real teammate with one goal: get real team work done based on your responsibilities.") ||
		!strings.Contains(prompt, "Be proactive, take initiative, understand the business and your team, and execute your role in service of the company's goals.") {
		t.Fatalf("rendered base prompt should preserve role identity tag:\n%s", prompt)
	}
}

func TestReplaceTaggedSection_ReplacesOnlyRequestedTag(t *testing.T) {
	prompt := "<identity>\nold\n</identity>\n\n<environment>\nkeep\n</environment>"

	got := replaceTaggedSection(prompt, "identity", "new")

	if !strings.Contains(got, "<identity>\nnew\n</identity>") {
		t.Fatalf("identity tag not replaced:\n%s", got)
	}
	if !strings.Contains(got, "<environment>\nkeep\n</environment>") {
		t.Fatalf("environment tag should be unchanged:\n%s", got)
	}
}

func TestAppendTaggedSection_PreservesExistingContent(t *testing.T) {
	prompt := "<environment>\nstatic environment\n</environment>"

	got := appendTaggedSection(prompt, "environment", "This sandbox has 4 CPU cores, 8 GB of memory, and 40 GB of disk available.")

	if !strings.Contains(got, "static environment\n\nThis sandbox has 4 CPU cores, 8 GB of memory, and 40 GB of disk available.") {
		t.Fatalf("environment tag did not preserve and append content:\n%s", got)
	}
}

func TestRenderEnvironmentContextUsesDefaultSandboxSizeWithoutTemplate(t *testing.T) {
	db := connectCompileTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Environment Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "Runtime Agent",
		SandboxStrategy: "always_on",
		Status:          "active",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	snapshotID := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.2.1-amd64"
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &orgID,
		AgentID:                &agent.ID,
		SnapshotID:             &snapshotID,
		ProviderID:             "microsandbox",
		ExternalID:             "environment-test",
		RuntimeURL:             "https://7080-environment-test.preview.usehivy.com",
		EncryptedRuntimeSecret: []byte("secret"),
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	got := renderEnvironmentContext(context.Background(), db, &agent)
	want := "This sandbox has 1 CPU core, 2 GB of memory, and 10 GB of disk available."
	if !strings.Contains(got, want) {
		t.Fatalf("environment context=%q, want %q", got, want)
	}
	for _, want := range []string{
		"https://<port>-environment-test.preview.usehivy.com",
		"Configured user-facing preview ports: 3000, 5173, 8000, 8080.",
		"Strict requirement: never share localhost, 127.0.0.1, or any other sandbox-local URL with the user",
		"make sure the app or server is running in the background",
		"include the public preview URL in your response",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("environment context missing preview guidance %q:\n%s", want, got)
		}
	}
}

func TestRenderEnvironmentContextUsesAgentSandboxSize(t *testing.T) {
	db := connectCompileTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Environment Size Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "Runtime Agent",
		SandboxStrategy: "always_on",
		SandboxSize:     "xlarge",
		Status:          "active",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	snapshotID := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.2.1-amd64"
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &orgID,
		AgentID:                &agent.ID,
		SnapshotID:             &snapshotID,
		ProviderID:             "microsandbox",
		ExternalID:             "environment-size-test",
		RuntimeURL:             "http://runtime.test",
		EncryptedRuntimeSecret: []byte("secret"),
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	got := renderEnvironmentContext(context.Background(), db, &agent)
	want := "This sandbox has 8 CPU cores, 16 GB of memory, and 60 GB of disk available."
	if !strings.Contains(got, want) {
		t.Fatalf("environment context=%q, want %q", got, want)
	}
}

func TestSandboxPreviewEnvironmentContextUsesFallbackDomain(t *testing.T) {
	got := sandboxPreviewEnvironmentContext(model.Sandbox{
		ProviderID: "microsandbox",
		ExternalID: "fallback-test",
		RuntimeURL: "http://runtime.test",
	})

	if !strings.Contains(got, "https://<port>-fallback-test.preview.usehivy.com") {
		t.Fatalf("preview context should fall back to production preview domain:\n%s", got)
	}
}

func TestSandboxPreviewEnvironmentContextSkipsNonMicrosandbox(t *testing.T) {
	got := sandboxPreviewEnvironmentContext(model.Sandbox{
		ProviderID: "docker",
		ExternalID: "container-test",
		RuntimeURL: "http://localhost:7080",
	})

	if got != "" {
		t.Fatalf("preview context should skip non-microsandbox provider, got %q", got)
	}
}

func TestPreviewBaseDomainFromRuntimeURL(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		sandboxID string
		want      string
	}{
		{
			name:      "microsandbox preview URL",
			rawURL:    "https://7080-sbx-test.preview.usehivy.com/healthz",
			sandboxID: "sbx-test",
			want:      "preview.usehivy.com",
		},
		{
			name:      "custom preview domain",
			rawURL:    "https://3000-sbx-test.preview.example.com",
			sandboxID: "sbx-test",
			want:      "preview.example.com",
		},
		{
			name:      "localhost is not a preview domain",
			rawURL:    "http://localhost:7080",
			sandboxID: "sbx-test",
			want:      "",
		},
		{
			name:      "ip address is not a preview domain",
			rawURL:    "http://127.0.0.1:7080",
			sandboxID: "sbx-test",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := previewBaseDomainFromRuntimeURL(tt.rawURL, tt.sandboxID); got != tt.want {
				t.Fatalf("previewBaseDomainFromRuntimeURL(%q, %q) = %q, want %q", tt.rawURL, tt.sandboxID, got, tt.want)
			}
		})
	}
}

func TestResourcePhrases(t *testing.T) {
	if got := cpuPhrase(1); got != "1 CPU core" {
		t.Fatalf("cpuPhrase(1) = %q", got)
	}
	if got := cpuPhrase(2); got != "2 CPU cores" {
		t.Fatalf("cpuPhrase(2) = %q", got)
	}
	if got := gbPhrase(1); got != "1 GB" {
		t.Fatalf("gbPhrase(1) = %q", got)
	}
	if got := gbPhrase(8); got != "8 GB" {
		t.Fatalf("gbPhrase(8) = %q", got)
	}
}

func TestDefaultCompanyPromptUsesSentences(t *testing.T) {
	got := defaultCompanyPrompt(model.Org{
		Name:        "ExampleCo",
		Website:     "https://example.com",
		Description: "Builds field-service software",
	})

	for _, want := range []string{
		"You are a core member of the company ExampleCo.",
		"Our main website is at https://example.com.",
		"This is what we do: Builds field-service software.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("default company prompt missing %q: %q", want, got)
		}
	}
	for _, oldStyle := range []string{"Company name:", "Website:", "Company description:"} {
		if strings.Contains(got, oldStyle) {
			t.Fatalf("default company prompt still uses key/value style %q: %q", oldStyle, got)
		}
	}
}
