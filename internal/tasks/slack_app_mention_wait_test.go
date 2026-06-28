package tasks

import (
	"errors"
	"strings"
	"testing"
)

func TestReadSlackFinalTextRequiresFinalText(t *testing.T) {
	text, err := readSlackFinalText(t.Context(), strings.NewReader("event: final\ndata: {\"scope\":\"main\",\"text\":\"Done\"}\n\n"))
	if err != nil {
		t.Fatalf("read final text: %v", err)
	}
	if text != "Done" {
		t.Fatalf("text=%q, want Done", text)
	}
}

func TestReadSlackFinalTextRetriesWhenNoFinalText(t *testing.T) {
	_, err := readSlackFinalText(t.Context(), strings.NewReader("event: turn_completed\ndata: {}\n\n"))
	if !errors.Is(err, errSlackNoFinalText) {
		t.Fatalf("err=%v, want errSlackNoFinalText", err)
	}
}

func TestReadSlackFinalTextIgnoresSubagentFinal(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		"event: final\n",
		"data: {\"scope\":\"subagent\",\"text\":\"Subagent answer\"}\n\n",
		"event: turn_completed\n",
		"data: {\"scope\":\"subagent\"}\n\n",
		"event: final\n",
		"data: {\"scope\":\"main\",\"text\":\"Main answer\"}\n\n",
	}, ""))
	text, err := readSlackFinalText(t.Context(), body)
	if err != nil {
		t.Fatalf("read final text: %v", err)
	}
	if text != "Main answer" {
		t.Fatalf("text=%q, want Main answer", text)
	}
}

func TestReadSlackFinalTextRequiresMainScope(t *testing.T) {
	body := strings.NewReader("event: final\ndata: {\"text\":\"Unscoped answer\"}\n\nevent: turn_completed\ndata: {}\n\n")
	_, err := readSlackFinalText(t.Context(), body)
	if !errors.Is(err, errSlackNoFinalText) {
		t.Fatalf("err=%v, want errSlackNoFinalText", err)
	}
}

func TestReadSlackFinalTextIgnoresSubagentError(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		"event: error\n",
		"data: {\"scope\":\"subagent\",\"message\":\"Subagent failed\"}\n\n",
		"event: final\n",
		"data: {\"scope\":\"main\",\"text\":\"Main recovered\"}\n\n",
	}, ""))
	text, err := readSlackFinalText(t.Context(), body)
	if err != nil {
		t.Fatalf("read final text: %v", err)
	}
	if text != "Main recovered" {
		t.Fatalf("text=%q, want Main recovered", text)
	}
}
