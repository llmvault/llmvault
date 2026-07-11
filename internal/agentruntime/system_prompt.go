package agentruntime

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	runtimeapi "github.com/usehivy/hivy/internal/sandboxruntime"
	"github.com/usehivy/hivy/internal/skills"
)

type PromptSections struct {
	Base          string
	Instructions  PromptSection
	SubAgents     PromptSection
	Company       PromptSection
	Communication PromptSection
	SkillHint     string
}

type PromptSection struct {
	Title   string
	Tag     string
	Content string
}

type SystemPromptConfig = runtimeapi.SystemPromptConfig
type SystemPromptSegment = runtimeapi.SystemPromptSegment
type StaticPromptSegment = runtimeapi.StaticPromptSegment

//go:embed system_prompt.md
var agentBaseSystemPrompt string

func buildPromptSections(ctx context.Context, db *gorm.DB, agent *model.Agent, description, modelID string) PromptSections {
	var org model.Org
	var hasOrg bool
	if agent != nil && agent.OrgID != nil && db != nil {
		if err := db.WithContext(ctx).Where("id = ?", *agent.OrgID).First(&org).Error; err == nil {
			hasOrg = true
		}
	}

	base := renderBaseSystemPrompt(agent, org, hasOrg, description)
	profile := resolveCommunicationProfile(modelID)
	fragments := PromptSections{
		Base: base,
		Communication: PromptSection{
			Title:   "Communication",
			Tag:     "communication",
			Content: profile.Content,
		},
	}
	fragments.SkillHint = renderSkillHint(ctx, db, agent)
	if instructions := effectiveAgentInstructions(ctx, db, agent); instructions != "" {
		fragments.Instructions = PromptSection{Title: "Instructions", Tag: "instructions", Content: instructions}
	}
	if subAgents := renderSubAgentRoutingSection(ctx, db, agent); subAgents != "" {
		fragments.SubAgents = PromptSection{Title: "Available subagents", Tag: "subagents", Content: subAgents}
	}
	if hasOrg {
		companyContent := strings.TrimSpace(org.PromptCompany)
		if companyContent == "" {
			companyContent = defaultCompanyPrompt(org)
		}
		if companyContent != "" {
			fragments.Company = PromptSection{Title: "About the company", Tag: "company", Content: companyContent}
		}
	}
	return fragments
}

func buildAgentSystemPrompt(ctx context.Context, fragments PromptSections) SystemPromptConfig {
	basePrompt := strings.TrimSpace(fragments.Base)
	if basePrompt == "" {
		basePrompt = renderBaseSystemPrompt(nil, model.Org{}, false, "")
	}
	cacheable := []SystemPromptSegment{
		staticPromptSegment("", basePrompt),
	}
	for _, fragment := range []PromptSection{
		fragments.Instructions,
		fragments.SubAgents,
		fragments.Company,
		fragments.Communication,
	} {
		if strings.TrimSpace(fragment.Content) == "" {
			continue
		}
		cacheable = append(cacheable, staticPromptSegment(fragment.Title, taggedPromptSection(fragment)))
	}

	dynamic := []SystemPromptSegment{
		mcpToolsPromptSegment(),
	}
	if hint := strings.TrimSpace(fragments.SkillHint); hint != "" {
		dynamic = append([]SystemPromptSegment{staticPromptSegment("Available skills", hint)}, dynamic...)
	}

	return SystemPromptConfig{
		CacheableSegments: &cacheable,
		DynamicSegments:   &dynamic,
	}
}

func renderSubAgentRoutingSection(ctx context.Context, db *gorm.DB, agent *model.Agent) string {
	specs, err := loadCatalogSubAgents(ctx, db, agent)
	if err != nil || len(specs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(specs))
	for key := range specs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := []string{
		"Delegate independent work when it materially improves speed or coverage. Give each subagent a clear goal and relevant context.",
		"",
		"Configured subagents:",
	}
	for _, key := range keys {
		spec := specs[key]
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = key
		}
		description := strings.TrimSpace(spec.Description)
		if description == "" {
			description = "Use according to this subagent's configured instructions."
		}
		lines = append(lines, fmt.Sprintf("- `%s` (%s). When to use: %s", key, name, ensureSentence(description)))
	}
	return strings.Join(lines, "\n")
}

func taggedPromptSection(section PromptSection) string {
	content := strings.TrimSpace(section.Content)
	if content == "" || strings.TrimSpace(section.Tag) == "" {
		return content
	}
	return wrapXMLTag(section.Tag, content)
}

func wrapXMLTag(tag, content string) string {
	tag = strings.TrimSpace(tag)
	content = strings.TrimSpace(content)
	if tag == "" || content == "" {
		return content
	}
	return "<" + tag + ">\n" + content + "\n</" + tag + ">"
}

func staticPromptSegment(title, content string) SystemPromptSegment {
	segment := SystemPromptSegment{}
	mustBuildPromptSegment(segment.FromSystemPromptSegment0(runtimeapi.SystemPromptSegment0{
		Type: runtimeapi.StaticText,
		Config: StaticPromptSegment{
			Title:   ptrNonEmpty(strings.TrimSpace(title)),
			Content: ptrNonEmpty(strings.TrimSpace(content)),
		},
	}))
	return segment
}

// renderSkillHint builds a static "Available skills" prompt hint from the
// agent's DB-backed skills. skill_view loads a selected skill; this hint is the
// complete skill inventory, so no skills_list MCP tool is needed. Returns ""
// when the agent has no skills.
func renderSkillHint(ctx context.Context, db *gorm.DB, agent *model.Agent) string {
	summaries, err := skills.AgentSkillSummaries(ctx, db, agent)
	if err != nil || len(summaries) == 0 {
		return ""
	}
	lines := []string{
		"When a skill matches the request, load it with `skill_view`.",
	}
	for _, summary := range summaries {
		description := strings.TrimSpace(summary.Description)
		if description == "" {
			lines = append(lines, fmt.Sprintf("- %s", summary.Name))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", summary.Name, description))
	}
	return strings.Join(lines, "\n")
}

func mcpToolsPromptSegment() SystemPromptSegment {
	segment := SystemPromptSegment{}
	mustBuildPromptSegment(segment.FromSystemPromptSegment2(runtimeapi.SystemPromptSegment2{
		Type: runtimeapi.McpTools,
		Config: runtimeapi.ListPromptSegment{
			Title:        ptrString("Available tools"),
			ItemTemplate: ptrString("- {name}"),
		},
	}))
	return segment
}

func mustBuildPromptSegment(err error) {
	if err != nil {
		panic(fmt.Sprintf("build agent system prompt segment: %v", err))
	}
}

func ptrString(value string) *string {
	return &value
}

func ptrNonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func agentDisplayName(agent *model.Agent) string {
	if agent != nil && strings.TrimSpace(agent.Name) != "" {
		return strings.TrimSpace(agent.Name)
	}
	return managedAgentName
}

func defaultCompanyPrompt(org model.Org) string {
	name := strings.TrimSpace(org.Name)
	website := strings.TrimSpace(org.Website)
	description := strings.TrimSpace(org.Description)

	if name == "" && website == "" && description == "" {
		return ""
	}
	var lines []string
	if name != "" {
		lines = append(lines, "Organization: "+ensureSentence(name))
	}
	if website != "" {
		lines = append(lines, "Website: "+ensureSentence(website))
	}
	if description != "" {
		lines = append(lines, "Description: "+ensureSentence(description))
	}
	return strings.Join(lines, "\n")
}
