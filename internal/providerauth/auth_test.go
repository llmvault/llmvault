package providerauth

import (
	"context"
	"net/http"
	"testing"
)

func TestAttachAppliesConfiguredAuthentication(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		header string
		value  string
	}{
		{name: "bearer", scheme: "bearer", header: "Authorization", value: "Bearer test-key"},
		{name: "api key", scheme: "api-key", header: "api-key", value: "test-key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://provider.test/v1/chat/completions", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			Attach(req, test.scheme, []byte("test-key"))
			if got := req.Header.Get(test.header); got != test.value {
				t.Fatalf("%s = %q, want %q", test.header, got, test.value)
			}
		})
	}
}
