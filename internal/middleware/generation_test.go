package middleware

import (
	"testing"
	"unicode/utf8"
)

func TestShouldCaptureProxyGeneration(t *testing.T) {
	if shouldCaptureProxyGeneration(&TokenClaims{TokenType: "agent_proxy"}) {
		t.Fatal("agent runtime proxy calls should be captured from model_usage webhooks")
	}
	if !shouldCaptureProxyGeneration(&TokenClaims{TokenType: "embedding_proxy"}) {
		t.Fatal("non-agent proxy calls should still be captured by proxy middleware")
	}
	if shouldCaptureProxyGeneration(nil) {
		t.Fatal("nil claims should not be captured")
	}
}

func TestTruncateValidUTF8SanitizesProviderErrorBytes(t *testing.T) {
	got := truncateValidUTF8("prefix \x8b\x00 suffix", 1000)
	if !utf8.ValidString(got) {
		t.Fatalf("error message is not valid UTF-8: %q", got)
	}
	if got != "prefix ?? suffix" {
		t.Fatalf("sanitized message = %q, want %q", got, "prefix ?? suffix")
	}
}

func TestTruncateValidUTF8DoesNotSplitMultibyteRune(t *testing.T) {
	got := truncateValidUTF8("abé", 3)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated message is not valid UTF-8: %q", got)
	}
	if got != "ab?" {
		t.Fatalf("truncated message = %q, want %q", got, "ab?")
	}
}
