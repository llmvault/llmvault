package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
)

func githubConn() *model.Connection {
	return &model.Connection{Integration: model.Integration{Provider: "github"}}
}

func notionConn() *model.Connection {
	return &model.Connection{Integration: model.Integration{Provider: "notion"}}
}

func TestValidateScope(t *testing.T) {
	// discovery is nil: the membership check is best-effort and skipped, so
	// these cases exercise the parse + rag_scopable type checks only.
	h := &RAGSourceHandler{catalog: catalog.Global()}

	cases := []struct {
		name       string
		conn       *model.Connection
		config     string
		wantStatus int
	}{
		{"nil connection is a no-op", nil, `{"scope":{"resource_type":"repository","items":[{"id":"a/b"}]}}`, 0},
		{"no scope key is fine", githubConn(), `{"repo_owner":"acme"}`, 0},
		{"empty config is fine", githubConn(), ``, 0},
		{"valid scopable type passes (best-effort, nil discovery)", githubConn(),
			`{"scope":{"resource_type":"repository","items":[{"id":"acme/widget"}]}}`, 0},
		{"non-scopable resource type is rejected", githubConn(),
			`{"scope":{"resource_type":"issue","items":[{"id":"1"}]}}`, http.StatusUnprocessableEntity},
		{"empty resource_type is rejected", githubConn(),
			`{"scope":{"resource_type":"","items":[{"id":"1"}]}}`, http.StatusUnprocessableEntity},
		{"scope with no items is rejected", githubConn(),
			`{"scope":{"resource_type":"repository","items":[]}}`, http.StatusUnprocessableEntity},
		{"notion mixed pages + databases via per-item type", notionConn(),
			`{"scope":{"items":[{"id":"p1","type":"page"},{"id":"d1","type":"database"}]}}`, 0},
		{"notion per-item type falls back to top-level resource_type", notionConn(),
			`{"scope":{"resource_type":"page","items":[{"id":"p1"}]}}`, 0},
		{"item with a non-scopable per-item type is rejected", notionConn(),
			`{"scope":{"items":[{"id":"p1","type":"page"},{"id":"x","type":"bogus"}]}}`, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, msg := h.validateScope(context.Background(), tc.conn, json.RawMessage(tc.config))
			if status != tc.wantStatus {
				t.Fatalf("status = %d (%q), want %d", status, msg, tc.wantStatus)
			}
		})
	}
}
