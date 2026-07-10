package agentruntime

import (
	"context"
	"strings"
	"testing"
)

func TestResolveCommunicationProfile_UsesNaturalExtremelyConcisePolicy(t *testing.T) {
	profile := resolveCommunicationProfile("deepseek-v4-flash")
	if profile.ID != naturalExtremelyConciseProfileID {
		t.Fatalf("profile ID = %q", profile.ID)
	}
	for _, want := range []string{
		"plain language",
		"at most two short paragraphs",
		"technical detail or another format",
		"Skip filler.",
	} {
		if !strings.Contains(profile.Content, want) {
			t.Fatalf("profile missing %q: %q", want, profile.Content)
		}
	}
}

func TestBuildPromptSections_AddsCommunicationProfileAsSeparateSegment(t *testing.T) {
	fragments := buildPromptSections(context.Background(), nil, nil, "", "step-3.7-flash")
	if fragments.Communication.Title != "Communication" || fragments.Communication.Tag != "communication" {
		t.Fatalf("communication section = %#v", fragments.Communication)
	}
	if fragments.Communication.Content != resolveCommunicationProfile("step-3.7-flash").Content {
		t.Fatalf("communication content = %q", fragments.Communication.Content)
	}
}
