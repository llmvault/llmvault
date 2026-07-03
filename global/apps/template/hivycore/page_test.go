package hivycore

import "testing"

func TestFrameAncestorsFrom(t *testing.T) {
	cases := []struct {
		name      string
		launchURL string
		want      string
	}{
		{"https origin", "https://app.usehivy.com/w/apps/abc/launch", "frame-ancestors 'self' https://app.usehivy.com"},
		{"origin with port", "http://localhost:3000/w/apps/abc/launch", "frame-ancestors 'self' http://localhost:3000"},
		{"empty", "", "frame-ancestors 'self'"},
		{"no scheme", "app.usehivy.com/launch", "frame-ancestors 'self'"},
		{"garbage", "://not a url", "frame-ancestors 'self'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := frameAncestorsFrom(tc.launchURL); got != tc.want {
				t.Fatalf("frameAncestorsFrom(%q) = %q, want %q", tc.launchURL, got, tc.want)
			}
		})
	}
}
