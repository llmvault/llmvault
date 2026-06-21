package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestBuildAgentSystemPrompt_IncludesInstructionsAsCacheableSegment(t *testing.T) {
	orgID := uuid.New()
	description := "Coordinates incident response."
	instructions := "Always confirm production impact before proposing mitigation."
	agent := &model.Agent{
		ID:           uuid.New(),
		OrgID:        &orgID,
		Name:         "Incident Agent",
		Description:  &description,
		Instructions: &instructions,
	}

	fragments := buildPromptSections(context.Background(), nil, agent, description, "")
	prompt := buildAgentSystemPrompt(context.Background(), fragments)
	cacheable := requireCacheableSegments(t, prompt)
	dynamic := requireDynamicSegments(t, prompt)

	var found bool
	for _, segment := range cacheable {
		static := requireStaticPromptSegment(t, segment)
		title := ""
		if static.Title != nil {
			title = *static.Title
		}
		content := requirePromptString(t, static.Content)
		if title == "Instructions" {
			found = true
			if content != "<instructions>\n"+instructions+"\n</instructions>" {
				t.Fatalf("instruction content = %q, want %q", content, instructions)
			}
		}
	}
	if !found {
		t.Fatalf("cacheable prompt missing Instructions segment")
	}
	for _, segment := range dynamic {
		raw := fmt.Sprintf("%#v", segment)
		if strings.Contains(raw, instructions) {
			t.Fatalf("agent instructions leaked into dynamic segment: %s", raw)
		}
	}
}
