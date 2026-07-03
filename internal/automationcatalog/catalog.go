package automationcatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultTriggerDir  = "global/triggers"
	DefaultScheduleDir = "global/schedules"
	KindTrigger        = "trigger"
	KindSchedule       = "schedule"

	supportedVersion = 1
)

type CatalogItem struct {
	Kind         string          `json:"kind"`
	Version      int             `json:"version"`
	Slug         string          `json:"slug"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Category     string          `json:"category"`
	Enabled      bool            `json:"enabled"`
	Official     bool            `json:"official"`
	Integration  IntegrationSpec `json:"integration"`
	Plugins      PluginsSpec     `json:"plugins"`
	Trigger      *TriggerSpec    `json:"trigger,omitempty"`
	Schedule     *ScheduleSpec   `json:"schedule,omitempty"`
	Resources    map[string]any  `json:"resources,omitempty"`
	Limits       map[string]any  `json:"limits,omitempty"`
	Install      InstallSpec     `json:"install"`
	Instructions string          `json:"instructions"`
}

type IntegrationSpec struct {
	Provider      string `json:"provider"`
	Required      bool   `json:"required"`
	ManualWebhook bool   `json:"manual_webhook,omitempty"`
}

type PluginsSpec struct {
	Required    []string `json:"required"`
	Recommended []string `json:"recommended"`
}

type TriggerSpec struct {
	Key      string          `json:"key"`
	Defaults TriggerDefaults `json:"defaults"`
}

type TriggerDefaults struct {
	Value        string `json:"value"`
	Instructions string `json:"instructions"`
}

type ScheduleSpec struct {
	Kind               string `json:"kind"`
	Cron               string `json:"cron"`
	Timezone           string `json:"timezone"`
	SuggestedTimeLabel string `json:"suggested_time_label"`
}

type InstallSpec struct {
	DefaultAgent   string `json:"default_agent"`
	DefaultChannel string `json:"default_channel,omitempty"`
	Enabled        bool   `json:"enabled"`
	Instructions   string `json:"instructions"`
}

func LoadTriggers(dir string) ([]CatalogItem, error) {
	if strings.TrimSpace(dir) == "" {
		dir = DefaultTriggerDir
	}
	return loadItems(dir, "trigger.json", KindTrigger)
}

func LoadSchedules(dir string) ([]CatalogItem, error) {
	if strings.TrimSpace(dir) == "" {
		dir = DefaultScheduleDir
	}
	return loadItems(dir, "schedule.json", KindSchedule)
}

func loadItems(dir, manifestName, kind string) ([]CatalogItem, error) {
	resolved, err := resolveDir(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("read automation catalog dir %q: %w", resolved, err)
	}
	items := make([]CatalogItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		item, err := loadItem(filepath.Join(resolved, entry.Name()), manifestName, kind)
		if err != nil {
			return nil, err
		}
		if item.Slug != entry.Name() {
			return nil, fmt.Errorf("catalog item %q must use folder name as slug", item.Slug)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Slug < items[j].Slug
	})
	return items, nil
}

func loadItem(dir, manifestName, kind string) (CatalogItem, error) {
	manifestPath := filepath.Join(dir, manifestName)
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return CatalogItem{}, fmt.Errorf("read automation catalog item %q: %w", manifestPath, err)
	}
	var item CatalogItem
	if err := json.Unmarshal(body, &item); err != nil {
		return CatalogItem{}, fmt.Errorf("parse automation catalog item %q: %w", manifestPath, err)
	}
	item.Kind = kind
	if err := item.loadInstructions(dir, kind); err != nil {
		return CatalogItem{}, err
	}
	if err := item.validate(kind); err != nil {
		return CatalogItem{}, err
	}
	return item, nil
}

func (item *CatalogItem) loadInstructions(dir, kind string) error {
	switch kind {
	case KindTrigger:
		if item.Trigger == nil {
			return nil
		}
		instructions, err := readInstructions(dir, item.Trigger.Defaults.Instructions)
		if err != nil {
			return fmt.Errorf("load trigger instructions for %q: %w", item.Slug, err)
		}
		item.Trigger.Defaults.Instructions = instructions
		item.Instructions = instructions
		return nil
	case KindSchedule:
		instructions, err := readInstructions(dir, item.Install.Instructions)
		if err != nil {
			return fmt.Errorf("load instructions for %q: %w", item.Slug, err)
		}
		item.Instructions = instructions
		return nil
	default:
		return nil
	}
}

func (item CatalogItem) validate(kind string) error {
	switch {
	case item.Version != supportedVersion:
		return fmt.Errorf("catalog item %q has unsupported version %d", item.Slug, item.Version)
	case strings.TrimSpace(item.Slug) == "":
		return fmt.Errorf("catalog item slug is required")
	case strings.TrimSpace(item.Name) == "":
		return fmt.Errorf("catalog item %q name is required", item.Slug)
	case strings.TrimSpace(item.Description) == "":
		return fmt.Errorf("catalog item %q description is required", item.Slug)
	case strings.TrimSpace(item.Category) == "":
		return fmt.Errorf("catalog item %q category is required", item.Slug)
	case strings.TrimSpace(item.Integration.Provider) == "":
		return fmt.Errorf("catalog item %q integration.provider is required", item.Slug)
	}
	switch kind {
	case KindTrigger:
		return item.validateTrigger()
	case KindSchedule:
		return item.validateSchedule()
	default:
		return fmt.Errorf("catalog item %q has unsupported kind %q", item.Slug, kind)
	}
}

func (item CatalogItem) validateTrigger() error {
	if item.Trigger == nil {
		return fmt.Errorf("catalog trigger %q trigger config is required", item.Slug)
	}
	if strings.TrimSpace(item.Trigger.Key) == "" {
		return fmt.Errorf("catalog trigger %q key is required", item.Slug)
	}
	// Defaults.Value is optional: some templates derive the trigger value at
	// install time (e.g. github-mention uses the selected repository).
	if strings.TrimSpace(item.Trigger.Defaults.Instructions) == "" {
		return fmt.Errorf("catalog trigger %q defaults.instructions is required", item.Slug)
	}
	return nil
}

func (item CatalogItem) validateSchedule() error {
	if item.Schedule == nil {
		return fmt.Errorf("catalog schedule %q schedule config is required", item.Slug)
	}
	if item.Schedule.Kind != "cron" {
		return fmt.Errorf("catalog schedule %q kind must be cron", item.Slug)
	}
	if strings.TrimSpace(item.Schedule.Cron) == "" {
		return fmt.Errorf("catalog schedule %q cron is required", item.Slug)
	}
	if strings.TrimSpace(item.Schedule.Timezone) == "" {
		return fmt.Errorf("catalog schedule %q timezone is required", item.Slug)
	}
	if strings.TrimSpace(item.Install.DefaultAgent) == "" {
		return fmt.Errorf("catalog schedule %q install.default_agent is required", item.Slug)
	}
	if strings.TrimSpace(item.Instructions) == "" {
		return fmt.Errorf("catalog schedule %q instructions are required", item.Slug)
	}
	return nil
}

func readInstructions(dir, rawPath string) (string, error) {
	instructionsPath, err := instructionsFile(dir, rawPath)
	if err != nil {
		return "", err
	}
	instructions, err := os.ReadFile(instructionsPath)
	if err != nil {
		return "", fmt.Errorf("read instructions: %w", err)
	}
	return string(instructions), nil
}

func instructionsFile(dir, rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("instructions path is required")
	}
	if filepath.IsAbs(rawPath) {
		return "", fmt.Errorf("instructions path must be relative")
	}
	cleaned := filepath.Clean(rawPath)
	full := filepath.Join(dir, cleaned)
	rel, err := filepath.Rel(dir, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("instructions path must stay within the catalog item")
	}
	return full, nil
}

func resolveDir(dir string) (string, error) {
	if filepath.IsAbs(dir) {
		return dir, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(cwd, dir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return filepath.Abs(dir)
}
