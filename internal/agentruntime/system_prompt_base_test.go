package agentruntime

import (
	"context"
	"strings"
	"testing"

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

func TestAgentIdentityOpeningUsesSentences(t *testing.T) {
	got := agentIdentityOpening("Ari", model.Org{Name: "ExampleCo"}, true, "Coordinates engineering work.")

	if want := "You are Ari, a real teammate embedded inside ExampleCo. Your role is described this way: Coordinates engineering work."; got != want {
		t.Fatalf("identity opening = %q, want %q", got, want)
	}
	for _, oldStyle := range []string{"Name:", "Role description:"} {
		if strings.Contains(got, oldStyle) {
			t.Fatalf("identity opening still uses key/value style %q: %q", oldStyle, got)
		}
	}
}
