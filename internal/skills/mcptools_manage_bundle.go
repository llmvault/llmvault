package skills

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

type parsedSkillToolBundle struct {
	Name                         string
	Description                  string
	HumanDescription             *string
	Category                     string
	Tags                         []string
	RequiredEnvironmentVariables []string
	Content                      string
	Files                        map[string]string
}

func parseSkillToolBundle(entrypoint string, suppliedFiles map[string]string) (*parsedSkillToolBundle, *mcp.CallToolResult) {
	if len(entrypoint) > maxSkillContentBytes {
		return nil, skillToolError(fmt.Sprintf("SKILL.md must be at most %d bytes", maxSkillContentBytes))
	}
	manifest, content, err := parseFrontmatter([]byte(entrypoint))
	if err != nil {
		return nil, skillToolError("SKILL.md has invalid YAML frontmatter")
	}
	if manifest == nil {
		return nil, skillToolError("SKILL.md must begin with YAML frontmatter containing name and description")
	}
	name, err := requiredManifestString(manifest, "name")
	if err != nil {
		return nil, skillToolError(err.Error())
	}
	description, err := requiredManifestString(manifest, "description")
	if err != nil {
		return nil, skillToolError(err.Error())
	}
	humanDescription, err := optionalManifestString(manifest, "human_description")
	if err != nil {
		return nil, skillToolError(err.Error())
	}
	category, err := optionalManifestString(manifest, "category")
	if err != nil {
		return nil, skillToolError(err.Error())
	}
	tags, err := manifestStringListStrict(manifest, "tags")
	if err != nil {
		return nil, skillToolError(err.Error())
	}
	envVars, err := manifestStringListStrict(manifest, "required_environment_variables")
	if err != nil {
		return nil, skillToolError(err.Error())
	}
	files := cleanSkillFiles(suppliedFiles)
	categoryValue := ""
	if category != nil {
		categoryValue = *category
	}
	if len(categoryValue) > 64 {
		return nil, skillToolError("category must be at most 64 characters")
	}
	if errResult := validateSkillFields(name, description, content, files, envVars, len(entrypoint)); errResult != nil {
		return nil, errResult
	}
	if slug := model.GenerateSlug(name); slug == "" || slug != name {
		return nil, skillToolError("SKILL.md frontmatter name must be a lowercase kebab-case skill identifier")
	}
	return &parsedSkillToolBundle{
		Name:                         name,
		Description:                  description,
		HumanDescription:             humanDescription,
		Category:                     categoryValue,
		Tags:                         normalizeStringList(tags),
		RequiredEnvironmentVariables: normalizeStringList(envVars),
		Content:                      content,
		Files:                        files,
	}, nil
}

func requiredManifestString(manifest map[string]any, key string) (string, error) {
	value, ok := manifest[key]
	if !ok {
		return "", fmt.Errorf("SKILL.md frontmatter %s is required", key)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("SKILL.md frontmatter %s must be a non-empty string", key)
	}
	return strings.TrimSpace(text), nil
}

func optionalManifestString(manifest map[string]any, key string) (*string, error) {
	value, ok := manifest[key]
	if !ok || value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("SKILL.md frontmatter %s must be a string", key)
	}
	text = strings.TrimSpace(text)
	return &text, nil
}

func manifestStringListStrict(manifest map[string]any, key string) ([]string, error) {
	value, ok := manifest[key]
	if !ok || value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		if stringItems, stringsOK := value.([]string); stringsOK {
			return stringItems, nil
		}
		return nil, fmt.Errorf("SKILL.md frontmatter %s must be an array of strings", key)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("SKILL.md frontmatter %s must contain only strings", key)
		}
		out = append(out, text)
	}
	return out, nil
}
