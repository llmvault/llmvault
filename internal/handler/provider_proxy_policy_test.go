package handler

import (
	"net/http"
	"testing"
)

func TestProviderProxyPolicy_DeniesGraphQLDestructiveMutations(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		body     string
		wantOp   string
	}{
		{
			name:     "railway remove mutation",
			provider: "railway",
			body:     `{"query":"mutation { deploymentRemove(id: \"dep\") }"}`,
			wantOp:   "deploymentRemove",
		},
		{
			name:     "linear delete mutation",
			provider: "linear",
			body:     `{"query":"mutation { issueDelete(id: \"issue\") { success } }"}`,
			wantOp:   "issueDelete",
		},
		{
			name:     "batched archive mutation",
			provider: "linear",
			body:     `[{"query":"query { viewer { id } }"},{"query":"mutation { projectArchive(id: \"project\") { success } }"}]`,
			wantOp:   "projectArchive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := providerProxyPolicyDecisionFor(tc.provider, http.MethodPost, "/graphql", []byte(tc.body))
			if !decision.Denied {
				t.Fatal("expected policy denial")
			}
			if decision.Operation != tc.wantOp {
				t.Fatalf("operation = %q, want %q", decision.Operation, tc.wantOp)
			}
		})
	}
}

func TestProviderProxyPolicy_AllowsSafeGraphQLMutations(t *testing.T) {
	body := []byte(`{"query":"mutation { issueUpdate(id: \"issue\", input: {title: \"Updated\"}) { issue { id } } }"}`)
	decision := providerProxyPolicyDecisionFor("linear", http.MethodPost, "/graphql", body)
	if decision.Denied {
		t.Fatalf("expected safe mutation to pass, denied %q", decision.Operation)
	}
}

func TestProviderProxyPolicy_DeniesRestDeleteArchiveAndRemove(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		method   string
		path     string
		body     string
	}{
		{name: "delete method", provider: "vercel", method: http.MethodDelete, path: "/v9/projects/app"},
		{name: "slack delete method path", provider: "slack", method: http.MethodPost, path: "/api/chat.delete"},
		{name: "slack archive method path", provider: "slack", method: http.MethodPost, path: "/api/conversations.archive"},
		{name: "notion trash flag", provider: "notion", method: http.MethodPatch, path: "/v1/pages/page", body: `{"in_trash":true}`},
		{name: "notion archived flag", provider: "notion", method: http.MethodPatch, path: "/v1/pages/page", body: `{"archived":true}`},
		{name: "vercel edge config delete operation", provider: "vercel", method: http.MethodPatch, path: "/v1/edge-config/id/items", body: `{"items":[{"operation":"delete","key":"old"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := providerProxyPolicyDecisionFor(tc.provider, tc.method, tc.path, []byte(tc.body))
			if !decision.Denied {
				t.Fatal("expected policy denial")
			}
		})
	}
}

func TestProviderProxyPolicy_AllowsSafeRestOperations(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		method   string
		path     string
		body     string
	}{
		{name: "slack history", provider: "slack", method: http.MethodGet, path: "/api/conversations.history"},
		{name: "notion update page properties", provider: "notion", method: http.MethodPatch, path: "/v1/pages/page", body: `{"properties":{"Name":{"title":[]}}}`},
		{name: "vercel create deployment", provider: "vercel", method: http.MethodPost, path: "/v13/deployments", body: `{"name":"app"}`},
		{name: "vercel edge config upsert", provider: "vercel", method: http.MethodPatch, path: "/v1/edge-config/id/items", body: `{"items":[{"operation":"upsert","key":"flag","value":true}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := providerProxyPolicyDecisionFor(tc.provider, tc.method, tc.path, []byte(tc.body))
			if decision.Denied {
				t.Fatalf("expected safe request to pass, denied %q", decision.Operation)
			}
		})
	}
}
