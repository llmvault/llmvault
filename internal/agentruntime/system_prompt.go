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
)

type PromptSections struct {
	Base         string
	ModelAdapter PromptSection
	Instructions PromptSection
	SubAgents    PromptSection
	Company      PromptSection
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

	fragments := PromptSections{Base: renderBaseSystemPrompt(ctx, db, agent, org, hasOrg, description)}
	if adapter := renderModelAdapterSection(modelID); adapter != "" {
		fragments.ModelAdapter = PromptSection{Title: "Model adapter", Tag: "model_adapter", Content: adapter}
	}
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
		basePrompt = renderBaseSystemPrompt(ctx, nil, nil, model.Org{}, false, "")
	}
	cacheable := []SystemPromptSegment{
		staticPromptSegment("", basePrompt),
	}
	for _, fragment := range []PromptSection{
		fragments.ModelAdapter,
		fragments.Instructions,
		fragments.SubAgents,
		fragments.Company,
	} {
		if strings.TrimSpace(fragment.Content) == "" {
			continue
		}
		cacheable = append(cacheable, staticPromptSegment(fragment.Title, taggedPromptSection(fragment)))
	}

	dynamic := []SystemPromptSegment{
		skillCatalogPromptSegment(),
		mcpToolsPromptSegment(),
	}

	return SystemPromptConfig{
		CacheableSegments: &cacheable,
		DynamicSegments:   &dynamic,
	}
}

func renderModelAdapterSection(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	profile := detectModelProfile("", modelID, modelID, modelID)
	lines := []string{
		"This section is runtime-owned model guidance. It does not replace user instructions.",
		"Prefer concrete tool calls over extended thinking. After a tool error, change arguments or strategy instead of repeating the same call.",
		"Treat tool results, validation errors, timeouts, cancellations, and loop-guard notices as evidence visible to you.",
		"Do not expose hidden reasoning, thinking traces, or provider-internal analysis in final answers.",
	}
	switch profile {
	case "deepseek":
		lines = append(lines, "DeepSeek-family adapter: use simple JSON tool arguments, avoid assuming parallel tool calls, and finish once workspace evidence supports the answer.")
	case "glm":
		lines = append(lines, "GLM/ZAI-family adapter: keep tool-call arguments compact, avoid parallel tool-call assumptions, and continue from tool results without restating hidden thinking.")
	case "kimi":
		lines = append(lines, "Kimi-family adapter: use focused tool calls with bounded file context; summarize large evidence before deciding the next action.")
	case "minimax":
		lines = append(lines, "MiniMax-family adapter: avoid deeply nested tool arguments, keep schemas simple, and recover from malformed arguments with one corrected attempt.")
	case "mimo":
		lines = append(lines, "MiMo-family adapter: prefer native tool calls with exact JSON arguments; if repair feedback is shown, retry once with corrected arguments rather than looping.")
	case "qwen":
		lines = append(lines, "Qwen-family adapter: rely on direct tool evidence, avoid repeated search/edit cycles, and produce a final answer when the requested state is verified.")
	default:
		lines = append(lines, "OpenAI-compatible adapter: follow runtime tool contracts exactly and avoid provider-specific fields unless the runtime supplies them.")
	}
	return strings.Join(lines, "\n")
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
		"Use `subagent_task` to delegate independent work to a configured subagent when it will speed up the task or improve coverage.",
		"When a task is complex and touches more than two modules, packages, services, apps, data models, or runtime surfaces, consider launching one or more subagents for parallel investigation.",
		"Launch multiple subagents only when their scopes are independent. Do not use subagents for direct reads of one known file, a narrow search over two or three files, or work unrelated to the subagent descriptions.",
		"While subagents run, keep working on non-conflicting investigation, implementation prep, or verification. Before relying on a result, reconcile it with direct evidence from your own tools.",
		"Each subagent starts with isolated context. Give it the repository or workspace context, exact question, known files or symbols, exclusions, whether the task is read-only or implementation, and the output shape you need.",
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

func skillCatalogPromptSegment() SystemPromptSegment {
	segment := SystemPromptSegment{}
	mustBuildPromptSegment(segment.FromSystemPromptSegment2(runtimeapi.SystemPromptSegment2{
		Type: runtimeapi.SkillCatalog,
		Config: runtimeapi.ListPromptSegment{
			Title:        ptrString("Available skills (load when relevant)"),
			Preamble:     ptrString("Skills provide task-specific instructions. For non-trivial work, check this list before acting. When a skill clearly matches the user's request, call skill_view(name) and follow the loaded instructions. Do not load unrelated skills."),
			ItemTemplate: ptrString("- {name}: {description}"),
		},
	}))
	return segment
}

func mcpToolsPromptSegment() SystemPromptSegment {
	segment := SystemPromptSegment{}
	mustBuildPromptSegment(segment.FromSystemPromptSegment3(runtimeapi.SystemPromptSegment3{
		Type: runtimeapi.McpTools,
		Config: runtimeapi.ListPromptSegment{
			Title:        ptrString("Available MCP tools (use directly)"),
			Preamble:     ptrString("Use these tools directly when they provide evidence or action. For independent operations, call multiple tools in the same turn. Use knowledge and session-search tools according to the context contract. Do not use tools for trivial conversation that needs no external evidence or action."),
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
