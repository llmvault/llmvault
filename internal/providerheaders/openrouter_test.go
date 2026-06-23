package providerheaders

import (
	"context"
	"net/http"
	"testing"
)

func TestIsOpenRouter(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		baseURL    string
		want       bool
	}{
		{name: "provider id", providerID: "openrouter", want: true},
		{name: "provider id case insensitive", providerID: "OpenRouter", want: true},
		{name: "base url", baseURL: "https://openrouter.ai/api/v1", want: true},
		{name: "other provider", providerID: "openai", baseURL: "https://api.openai.com/v1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOpenRouter(tt.providerID, tt.baseURL); got != tt.want {
				t.Fatalf("IsOpenRouter(%q, %q) = %v, want %v", tt.providerID, tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestApplyOpenRouter(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}

	ApplyOpenRouter(req)

	if got := req.Header.Get("HTTP-Referer"); got != "https://usehivy.com" {
		t.Fatalf("HTTP-Referer = %q, want https://usehivy.com", got)
	}
	if got := req.Header.Get("X-Title"); got != "Hivy" {
		t.Fatalf("X-Title = %q, want Hivy", got)
	}
}
