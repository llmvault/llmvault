package github

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLoadConfig_ScopeEnvelopeMultiOwner(t *testing.T) {
	raw := json.RawMessage(`{
		"scope": {
			"resource_type": "repository",
			"items": [
				{"id":"acme/widget","name":"widget"},
				{"id":"other-org/gadget","name":"gadget"}
			]
		},
		"include_prs": true
	}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"acme/widget", "other-org/gadget"}
	if !reflect.DeepEqual(cfg.FullNames, want) {
		t.Fatalf("FullNames=%v, want %v (multi-owner from scope)", cfg.FullNames, want)
	}
}

func TestLoadConfig_ScopeRejectsBareRepo(t *testing.T) {
	raw := json.RawMessage(`{"scope":{"resource_type":"repository","items":[{"id":"widget"}]}}`)
	if _, err := LoadConfig(raw); err == nil {
		t.Fatal("expected error: scope repo must be owner/repo form")
	}
}

func TestLoadConfig_LegacyStillWorks(t *testing.T) {
	raw := json.RawMessage(`{"repo_owner":"acme","repositories":["widget","gadget"]}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"acme/widget", "acme/gadget"}
	if !reflect.DeepEqual(cfg.FullNames, want) {
		t.Fatalf("FullNames=%v, want %v (legacy owner+repos)", cfg.FullNames, want)
	}
}

func TestLoadConfig_ScopeTakesPrecedenceOverLegacy(t *testing.T) {
	raw := json.RawMessage(`{
		"repo_owner":"legacy",
		"repositories":["ignored"],
		"scope":{"resource_type":"repository","items":[{"id":"acme/widget"}]}
	}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg.FullNames, []string{"acme/widget"}) {
		t.Fatalf("FullNames=%v, want [acme/widget] (scope wins)", cfg.FullNames)
	}
}
