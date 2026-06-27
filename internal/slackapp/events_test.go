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
