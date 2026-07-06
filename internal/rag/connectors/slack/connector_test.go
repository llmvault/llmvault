package slack

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"
)

func TestChannelMembershipFilter(t *testing.T) {
	channels := []SlackChannel{
		makeChannel("C1", "general", true, false),
		makeChannel("C2", "random", false, false),
		makeChannel("C3", "private", true, true),
	}
	archived := makeChannel("C4", "archived", true, false)
	archived.IsArchived = true
	channels = append(channels, archived)

	var filtered []SlackChannel
	for _, ch := range channels {
		if !ch.IsMember || ch.IsArchived {
			continue
		}
		filtered = append(filtered, ch)
	}

	if len(filtered) != 2 {
		t.Fatalf("expected 2 channels after filtering, got %d", len(filtered))
	}
	ids := make([]string, len(filtered))
	for i, ch := range filtered {
		ids[i] = ch.ID
	}
	sort.Strings(ids)
	if ids[0] != "C1" || ids[1] != "C3" {
		t.Errorf("unexpected filtered channels: %v", ids)
	}
}

func TestFetchMemberChannels_OnlyMembers(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(
		makeChannel("C1", "general", true, false),
		makeChannel("C2", "random", false, false),
		makeChannel("C3", "private", true, true),
	)

	c := newConnectorWithAPI(SlackConfig{ChannelNames: []string{"*"}, ChannelRegexEnabled: true}, fake)
	c.ctx = context.Background()
	chs, err := c.fetchMemberChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(chs) != 2 {
		t.Fatalf("expected 2 member channels, got %d", len(chs))
	}
}

func TestFetchMemberChannels_WithNameFilter(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(
		makeChannel("C1", "general", true, false),
		makeChannel("C2", "random", true, false),
		makeChannel("C3", "eng-team", true, false),
	)

	c := newConnectorWithAPI(SlackConfig{ChannelNames: []string{"general", "eng-team"}}, fake)
	c.ctx = context.Background()
	chs, err := c.fetchMemberChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(chs) != 2 {
		t.Fatalf("expected 2 filtered channels, got %d", len(chs))
	}
}

func TestRunnable_Run(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "Hello", TS: "1000.000001"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{ChannelNames: []string{"*"}, ChannelRegexEnabled: true}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	ch, err := c.Run(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)}, nil, time.Time{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	docs, _ := drainDocs(t, ch)
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
}

func TestRunnable_FinalCheckpoint(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "Hello", TS: "1000.000001"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{ChannelNames: []string{"*"}, ChannelRegexEnabled: true}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	ch, _ := c.Run(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)}, nil, time.Time{}, time.Now())
	drainDocs(t, ch)

	raw, err := c.FinalCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if raw == nil {
		t.Fatal("FinalCheckpoint returned nil")
	}
	var cp SlackCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		t.Fatalf("unmarshal final checkpoint: %v", err)
	}
	if cp.ChannelCompletionMap["C1"] == "" {
		t.Error("checkpoint should track channel progress")
	}
}
