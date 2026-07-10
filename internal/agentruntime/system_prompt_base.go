package agentruntime

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

func renderBaseSystemPrompt(agent *model.Agent, org model.Org, hasOrg bool, description string) string {
	prompt := agentBaseSystemPrompt
	return replaceTaggedSection(prompt, "identity", renderIdentityContext(agentDisplayName(agent), org, hasOrg, description))
}

func renderIdentityContext(agentName string, org model.Org, hasOrg bool, description string) string {
	lines := []string{fmt.Sprintf("You are %s, an AI agent running in Hivy's sandbox environment.", agentName)}
	if description = strings.TrimSpace(description); description != "" && description != managedAgentDescription {
		lines = append(lines, fmt.Sprintf("Your configured role: %s", ensureSentence(description)))
	}
	if hasOrg {
		if name := strings.TrimSpace(org.Name); name != "" {
			lines = append(lines, fmt.Sprintf("You are working for %s.", name))
		}
	}
	return strings.Join(lines, "\n")
}

func ensureSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch value[len(value)-1] {
	case '.', '!', '?':
		return value
	default:
		return value + "."
	}
}

func replaceTaggedSection(prompt, tag, content string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(prompt, open)
	if start < 0 {
		return prompt
	}
	contentStart := start + len(open)
	end := strings.Index(prompt[contentStart:], close)
	if end < 0 {
		return prompt
	}
	content = strings.TrimSpace(content)
	replacement := open
	if content != "" {
		replacement += "\n" + content + "\n"
	} else {
		replacement += "\n"
	}
	replacement += close
	return prompt[:start] + replacement + prompt[contentStart+end+len(close):]
}
