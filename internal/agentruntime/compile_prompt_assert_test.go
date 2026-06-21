package agentruntime

import (
	"strings"
	"testing"

	runtimeapi "github.com/usehivy/hivy/internal/sandboxruntime"
)

func requireCacheableSegments(t *testing.T, prompt SystemPromptConfig) []SystemPromptSegment {
	t.Helper()
	if prompt.CacheableSegments == nil {
		t.Fatal("cacheable segments is nil")
	}
	return *prompt.CacheableSegments
}

func requireDynamicSegments(t *testing.T, prompt SystemPromptConfig) []SystemPromptSegment {
	t.Helper()
	if prompt.DynamicSegments == nil {
		t.Fatal("dynamic segments is nil")
	}
	return *prompt.DynamicSegments
}

func requireStaticPromptSegment(t *testing.T, segment SystemPromptSegment) StaticPromptSegment {
	t.Helper()
	staticSegment, err := segment.AsSystemPromptSegment0()
	if err != nil {
		t.Fatalf("decode static prompt segment: %v", err)
	}
	if staticSegment.Type != "static_text" {
		t.Fatalf("static segment type = %q", staticSegment.Type)
	}
	return staticSegment.Config
}

func requireStaticPromptSegmentByTitle(t *testing.T, segments []SystemPromptSegment, title string) StaticPromptSegment {
	t.Helper()
	for _, segment := range segments {
		staticSegment := requireStaticPromptSegment(t, segment)
		gotTitle := ""
		if staticSegment.Title != nil {
			gotTitle = strings.TrimSpace(*staticSegment.Title)
		}
		if gotTitle == title {
			return staticSegment
		}
	}
	t.Fatalf("static prompt segment with title %q not found", title)
	return StaticPromptSegment{}
}

func requireDynamicContextSegmentType(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	dynamicSegment, err := segment.AsSystemPromptSegment1()
	if err != nil {
		t.Fatalf("decode dynamic context segment: %v", err)
	}
	return string(dynamicSegment.Type)
}

func requireDynamicContextSegment(t *testing.T, segment SystemPromptSegment) runtimeapi.SystemPromptSegment1 {
	t.Helper()
	dynamicSegment, err := segment.AsSystemPromptSegment1()
	if err != nil {
		t.Fatalf("decode dynamic context segment: %v", err)
	}
	return dynamicSegment
}

func requireListSegment2Type(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	listSegment, err := segment.AsSystemPromptSegment2()
	if err != nil {
		t.Fatalf("decode list segment: %v", err)
	}
	return string(listSegment.Type)
}

func requireListSegment3Type(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	listSegment, err := segment.AsSystemPromptSegment3()
	if err != nil {
		t.Fatalf("decode mcp tools segment: %v", err)
	}
	return string(listSegment.Type)
}

func requireListSegment3(t *testing.T, segment SystemPromptSegment) runtimeapi.SystemPromptSegment3 {
	t.Helper()
	listSegment, err := segment.AsSystemPromptSegment3()
	if err != nil {
		t.Fatalf("decode mcp tools segment: %v", err)
	}
	return listSegment
}

func requirePromptString(t *testing.T, value *string) string {
	t.Helper()
	if value == nil {
		t.Fatal("prompt string is nil")
	}
	return *value
}
