package slackapp

import "testing"

func TestDecodeInboundEventAppMention(t *testing.T) {
	raw := []byte(`{
		"type":"event_callback",
		"team_id":"T123",
		"event_id":"Ev123",
		"event":{
			"type":"app_mention",
			"channel":"C123",
			"user":"U123",
			"user_name":"dana",
			"display_name":"Dana",
			"text":"<@B123> summarize this thread",
			"ts":"1710000000.123456"
		}
	}`)
	event, ok, err := DecodeInboundEvent(raw)
	if err != nil {
		t.Fatalf("DecodeInboundEvent error: %v", err)
	}
	if !ok {
		t.Fatal("DecodeInboundEvent ignored app mention")
	}
	if event.ThreadTS != "1710000000.123456" || event.CleanText != "summarize this thread" {
		t.Fatalf("decoded event thread=%q text=%q", event.ThreadTS, event.CleanText)
	}
	if event.UserName != "dana" || event.DisplayName != "Dana" || event.Raw["user_name"] != "dana" {
		t.Fatalf("decoded user metadata name=%q display=%q raw=%#v", event.UserName, event.DisplayName, event.Raw)
	}
}

func TestDecodeInboundEventIgnoresPlainChannelMessage(t *testing.T) {
	raw := []byte(`{
		"type":"event_callback",
		"team_id":"T123",
		"event_id":"Ev124",
		"event":{
			"type":"message",
			"channel":"C123",
			"user":"U123",
			"text":"not in a thread",
			"ts":"1710000001.000000"
		}
	}`)
	_, ok, err := DecodeInboundEvent(raw)
	if err != nil {
		t.Fatalf("DecodeInboundEvent error: %v", err)
	}
	if ok {
		t.Fatal("plain channel message should be ignored")
	}
}

func TestDecodeReactionAddedEvent(t *testing.T) {
	raw := []byte(`{
		"type":"event_callback",
		"team_id":"T111",
		"event_id":"Ev111",
		"event_time":1599616881,
		"event":{
			"type":"reaction_added",
			"user":"W111",
			"reaction":"heart_eyes",
			"item":{"type":"message","channel":"C111","ts":"1599529504.000400"},
			"item_user":"W222",
			"event_ts":"1599616881.000800"
		}
	}`)
	event, ok, err := DecodeReactionAddedEvent(raw)
	if err != nil {
		t.Fatalf("DecodeReactionAddedEvent error: %v", err)
	}
	if !ok {
		t.Fatal("DecodeReactionAddedEvent ignored reaction_added")
	}
	if event.EventID != "Ev111" || event.Reaction != "heart_eyes" {
		t.Fatalf("decoded event id/reaction=%q/%q", event.EventID, event.Reaction)
	}
	if event.ItemType != "message" || event.ItemChannel != "C111" || event.ItemTS != "1599529504.000400" {
		t.Fatalf("decoded item=%q/%q/%q", event.ItemType, event.ItemChannel, event.ItemTS)
	}
	if event.Raw["event_id"] != "Ev111" {
		t.Fatalf("raw payload was not preserved: %#v", event.Raw)
	}
}
