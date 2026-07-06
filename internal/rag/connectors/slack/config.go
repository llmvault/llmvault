package slack

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/usehivy/hivy/internal/rag/connectors/scope"
)

// slackScopeResourceType is the resource-catalog key for a Slack channel scope.
const slackScopeResourceType = "slack_channel"

// SlackConfig is the connector-specific configuration stored in
// RAGSource.ConfigValue (JSONB). The user selects which channels to
// index and whether bot messages are included.
type SlackConfig struct {
	ChannelNames        []string `json:"channel_names,omitempty"`
	IncludeBotMessages  bool     `json:"include_bot_messages"`
	ChannelRegexEnabled bool     `json:"channel_regex_enabled"`

	// ChannelIDs is the set of channel IDs selected via the scope envelope
	// (config.scope). When non-empty it takes precedence over ChannelNames.
	// Not part of the JSON contract.
	ChannelIDs []string `json:"-"`
}

func LoadConfig(raw json.RawMessage) (SlackConfig, error) {
	cfg := SlackConfig{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return SlackConfig{}, fmt.Errorf("slack: parse config: %w", err)
		}
	}
	cfg.ChannelNames = normaliseChannelList(cfg.ChannelNames)

	sc, present, err := scope.Parse(raw)
	if err != nil {
		return SlackConfig{}, fmt.Errorf("slack: %w", err)
	}
	if present && sc.ResourceType == slackScopeResourceType {
		cfg.ChannelIDs = sc.IDs()
	}
	return cfg, nil
}

func normaliseChannelList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	dedup := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, name := range in {
		name = strings.TrimSpace(name)
		name = strings.TrimPrefix(name, "#")
		if name == "" {
			continue
		}
		if _, ok := dedup[name]; ok {
			continue
		}
		dedup[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// channelIsAllowed returns true if the channel should be indexed. Scope IDs
// (from the scope envelope) take precedence; otherwise the legacy name/regex
// filter applies. When nothing is configured, no channels are indexed
// (deny-by-default — an unscoped source ingests nothing).
func channelIsAllowed(channel SlackChannel, ids, names []string, regexEnabled bool) bool {
	if len(ids) > 0 {
		return channelIDInList(channel, ids)
	}
	if len(names) == 0 {
		return false
	}
	if regexEnabled {
		return channelMatchesRegex(channel, names)
	}
	return channelInList(channel, names)
}

func channelIDInList(channel SlackChannel, ids []string) bool {
	return slices.Contains(ids, channel.ID)
}

func channelInList(channel SlackChannel, names []string) bool {
	return slices.Contains(names, channel.Name)
}

func channelMatchesRegex(channel SlackChannel, patterns []string) bool {
	for _, p := range patterns {
		if match, _ := wildcardMatch(p, channel.Name); match {
			return true
		}
	}
	return false
}

func wildcardMatch(pattern, s string) (bool, error) {
	re, err := compileGlob(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

func compileGlob(pat string) (*regexp.Regexp, error) {
	raw := strings.ReplaceAll(regexp.QuoteMeta(pat), `\*`, ".*")
	return regexp.Compile(`^` + raw + `$`)
}
