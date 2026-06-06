package employeeruntime

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

func buildAvailableSpecialistsSection(agent *model.Employee, catalog *specialists.Catalog) PromptSection {
	if agent == nil || catalog == nil {
		return PromptSection{}
	}
	attached := map[string]bool{}
	for _, slug := range agent.AttachedSpecialists {
		slug = strings.TrimSpace(slug)
		if slug != "" {
			attached[slug] = true
		}
	}
	if len(attached) == 0 {
		return PromptSection{}
	}
	lines := []string{
		"Specialist agents are attached coworkers you can dispatch for focused work that benefits from separate execution.",
		"Use specialist_launch_task when a specialist is the better fit than doing the work directly. Choose based on the names, descriptions, and routing criteria below.",
		"Routing rubric:",
		"- Answer directly when the request is simple, low-risk, and can be satisfied from current context or a quick check.",
		"- Ask one focused clarification when the deliverable, target, scope, constraints, data source, timeframe, audience, or success criteria are missing. Do not launch a specialist to discover what the user meant.",
		"- Treat memories, knowledge base snippets, and past session context as valid evidence. If they supply the missing details for a build, research, debug, or investigation task, delegate without asking for the same clarification again.",
		"- Delegate when the objective is clear enough for the specialist to act independently and the work matches that specialist's criteria.",
		"- Dispatch heavy tasks that may consume computer resources for more than 30 seconds, or tasks that need disk or network reads and writes, when the request gives enough context to act.",
		"- Follow the specialist-specific delegation and clarification criteria in each description below.",
		"Give the specialist enough context to act independently, then use status tools to follow progress and report verified outcomes.",
		"For specialist work that will take more than a quick check, schedule a wake instead of repeatedly polling status.",
		"Attached specialist agents:",
	}
	for _, def := range catalog.List() {
		if !attached[def.Slug] {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", def.Name, def.Slug, strings.TrimSpace(def.Description)))
	}
	if len(lines) == 12 {
		return PromptSection{}
	}
	return PromptSection{
		Title:   "Specialist agents",
		Content: strings.Join(lines, "\n"),
	}
}
