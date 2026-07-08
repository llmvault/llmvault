package proxy

import "testing"

func TestParseResponseID(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "openrouter chat completion", body: `{"id":"gen-abc123","choices":[]}`, want: "gen-abc123"},
		{name: "sse chunk", body: `{"id":"gen-stream-1","object":"chat.completion.chunk","choices":[]}`, want: "gen-stream-1"},
		{name: "no id", body: `{"choices":[]}`, want: ""},
		{name: "id not string", body: `{"id":123}`, want: ""},
		{name: "malformed", body: `{"id":`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseResponseID([]byte(tt.body)); got != tt.want {
				t.Fatalf("parseResponseID(%s) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}
