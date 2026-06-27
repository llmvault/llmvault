package tasks

import (
	"errors"
	"strings"
	"testing"
)

func TestReadSlackFinalTextRequiresFinalText(t *testing.T) {
	text, err := readSlackFinalText(t.Context(), strings.NewReader("event: final\ndata: {\"text\":\"Done\"}\n\n"))
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
