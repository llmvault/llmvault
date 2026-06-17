package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func renderBaseSystemPrompt(ctx context.Context, db *gorm.DB, agent *model.Agent, org model.Org, hasOrg bool, description string) string {
	prompt := agentBaseSystemPrompt
	prompt = replaceTaggedSection(prompt, "identity", renderIdentityContext(agentDisplayName(agent), org, hasOrg, description))
	prompt = appendTaggedSection(prompt, "environment", renderEnvironmentContext(ctx, db, agent))
	return prompt
}

func renderIdentityContext(agentName string, org model.Org, hasOrg bool, description string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("You are %s, a real teammate with one goal: get real team work done based on your responsibilities.", agentName))
	lines = append(lines, "Be proactive, take initiative, understand the business and your team, and execute your role in service of the company's goals.")
	if description = strings.TrimSpace(description); description != "" && description != managedAgentDescription {
		lines = append(lines, fmt.Sprintf("Your role is described this way: %s", ensureSentence(description)))
	}
	if hasOrg {
		if name := strings.TrimSpace(org.Name); name != "" {
			lines = append(lines, fmt.Sprintf("You work at %s.", name))
		}
	}
	if hasOrg {
		description := strings.TrimSpace(org.PromptCompany)
		if description != "" {
			lines = append(lines, fmt.Sprintf("The company is described this way: %s", ensureSentence(description)))
		}
	}
	return strings.Join(lines, "\n")
}

func renderEnvironmentContext(ctx context.Context, db *gorm.DB, agent *model.Agent) string {
	if db == nil || agent == nil || agent.OrgID == nil {
		return ""
	}
	var sandbox model.Sandbox
	err := db.WithContext(ctx).
		Preload("SandboxTemplate").
		Where("org_id = ? AND agent_id = ? AND status IN ?", *agent.OrgID, agent.ID, []string{"running", "creating", "starting", "stopped"}).
		Order("created_at DESC").
		First(&sandbox).Error
	if err != nil {
		return ""
	}
	size := sandboxEnvironmentSize(ctx, db, agent, sandbox)
	if resources, ok := model.TemplateSizes[size]; ok {
		return fmt.Sprintf(
			"This sandbox has %s, %s of memory, and %s of disk available.",
			cpuPhrase(resources.CPU),
			gbPhrase(resources.Memory),
			gbPhrase(resources.Disk),
		)
	}
	if size != "" {
		return fmt.Sprintf("This sandbox is configured with the %s size.", size)
	}
	return ""
}

func sandboxEnvironmentSize(ctx context.Context, db *gorm.DB, agent *model.Agent, sandbox model.Sandbox) string {
	if agent != nil {
		return model.NormalizeTemplateSize(agent.SandboxSize)
	}
	if sandbox.SandboxTemplate != nil {
		if size := strings.TrimSpace(sandbox.SandboxTemplate.Size); size != "" {
			return size
		}
	}
	if agent == nil || agent.SandboxTemplateID == nil {
		return model.DefaultAgentSandboxSize
	}
	if agent.SandboxTemplate != nil {
		if size := strings.TrimSpace(agent.SandboxTemplate.Size); size != "" {
			return size
		}
	}
	if db == nil {
		return model.DefaultAgentSandboxSize
	}
	var tmpl model.SandboxTemplate
	if err := db.WithContext(ctx).Select("id", "size").Where("id = ?", *agent.SandboxTemplateID).First(&tmpl).Error; err == nil {
		if size := strings.TrimSpace(tmpl.Size); size != "" {
			return size
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ""
	}
	return model.DefaultAgentSandboxSize
}

func cpuPhrase(cpu int) string {
	if cpu == 1 {
		return "1 CPU core"
	}
	return fmt.Sprintf("%d CPU cores", cpu)
}

func gbPhrase(gb int) string {
	if gb == 1 {
		return "1 GB"
	}
	return fmt.Sprintf("%d GB", gb)
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

func appendTaggedSection(prompt, tag, content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return prompt
	}
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
	existing := strings.TrimSpace(prompt[contentStart : contentStart+end])
	combined := content
	if existing != "" {
		combined = existing + "\n\n" + content
	}
	return prompt[:start] + open + "\n" + combined + "\n" + close + prompt[contentStart+end+len(close):]
}
