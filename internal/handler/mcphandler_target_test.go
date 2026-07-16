package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestConnectionServerTarget(t *testing.T) {
	connectionID := uuid.New()
	tests := []struct {
		name     string
		wildcard string
		kind     string
		targeted bool
		wantErr  bool
	}{
		{name: "native", wildcard: "", targeted: false},
		{name: "nango connection", wildcard: "connection/" + connectionID.String(), kind: "connection", targeted: true},
		{name: "database connection", wildcard: "/database/" + connectionID.String() + "/", kind: "database", targeted: true},
		{name: "unknown kind", wildcard: "other/" + connectionID.String(), wantErr: true},
		{name: "invalid id", wildcard: "connection/nope", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("*", test.wildcard)
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
			kind, gotID, targeted, err := connectionServerTarget(r)
			if (err != nil) != test.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErr {
				return
			}
			if kind != test.kind || targeted != test.targeted {
				t.Fatalf("got kind=%q targeted=%v", kind, targeted)
			}
			if targeted && gotID != connectionID {
				t.Fatalf("got id %s, want %s", gotID, connectionID)
			}
		})
	}
}
