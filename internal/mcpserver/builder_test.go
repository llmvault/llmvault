package mcpserver

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestBuildServerWithNoScopes(t *testing.T) {
	server, err := BuildServer(
		context.Background(),
		&model.Token{Meta: model.JSON{model.TokenMetaType: model.TokenTypeAgentProxy}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("build server: %v", err)
	}
	if server == nil {
		t.Fatal("server is nil")
	}
}
