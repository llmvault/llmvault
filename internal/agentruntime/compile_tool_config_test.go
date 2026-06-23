package agentruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestCompileBashToolPassesCanvasRuntimeEnv(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Canvas Agent",
		Model: DefaultAgentModel,
		Tools: model.JSON{"bash": true},
	}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(def.Tools) != 1 {
		t.Fatalf("runtime tools = %#v, want one bash tool", def.Tools)
	}
	config, ok := def.Tools[0]["config"].(map[string]any)
	if !ok {
		t.Fatalf("bash config = %#v", def.Tools[0]["config"])
	}
	passthrough, ok := config["env_passthrough"].([]any)
	if !ok {
		t.Fatalf("bash env passthrough = %#v", config["env_passthrough"])
	}
	for _, key := range []string{
		AgentEnvPenpotCanvasURL,
		AgentEnvPenpotCanvasTeamID,
		AgentEnvPenpotCanvasProfileID,
		AgentEnvPenpotCanvasSessionJWT,
		AgentEnvPenpotCanvasMCPURL,
		AgentEnvCloudControlPlaneURL,
		AgentEnvAgentID,
		AgentEnvRuntimeSecret,
	} {
		if !containsAnyString(passthrough, key) {
			t.Fatalf("bash env passthrough missing %s: %#v", key, passthrough)
		}
	}
}

func TestCompileBashToolAddsCanvasEnvToCustomPassthrough(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Canvas Agent",
		Model: DefaultAgentModel,
		Tools: model.JSON{
			"bash": map[string]any{"env_passthrough": []any{"PATH", "HOME"}},
		},
	}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	config := def.Tools[0]["config"].(map[string]any)
	passthrough := config["env_passthrough"].([]any)
	if !containsAnyString(passthrough, AgentEnvPenpotCanvasSessionJWT) {
		t.Fatalf("custom bash env passthrough missing canvas env: %#v", passthrough)
	}
	if !containsAnyString(passthrough, AgentEnvRuntimeSecret) {
		t.Fatalf("custom bash env passthrough missing runtime secret: %#v", passthrough)
	}
}

func TestCompileImageGenerationTools(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Image Agent",
		Model: DefaultAgentModel,
		Tools: model.JSON{
			"generate_image":        true,
			"generate_vector_image": true,
		},
	}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(def.Tools) != 2 {
		t.Fatalf("runtime tools = %#v, want two image tools", def.Tools)
	}
	if def.Tools[0]["type"] != "builtin.generate_image" {
		t.Fatalf("first tool = %#v", def.Tools[0])
	}
	imageConfig := def.Tools[0]["config"].(map[string]any)
	if imageConfig["default_model"] != registry.DefaultRasterImageGenerationModelID || imageConfig["mode"] != "raster" {
		t.Fatalf("image config = %#v", imageConfig)
	}
	assertImageGenerationResultShape(t, imageConfig)
	if def.Tools[1]["type"] != "builtin.generate_vector_image" {
		t.Fatalf("second tool = %#v", def.Tools[1])
	}
	vectorConfig := def.Tools[1]["config"].(map[string]any)
	if vectorConfig["default_model"] != registry.DefaultVectorImageGenerationModelID || vectorConfig["mode"] != "vector" {
		t.Fatalf("vector config = %#v", vectorConfig)
	}
	assertImageGenerationResultShape(t, vectorConfig)
}

func assertImageGenerationResultShape(t *testing.T, config map[string]any) {
	t.Helper()
	shape, ok := config["result_shape"].(map[string]any)
	if !ok {
		t.Fatalf("result_shape = %#v", config["result_shape"])
	}
	if shape["type"] != "array" {
		t.Fatalf("result_shape type = %#v", shape)
	}
	items, ok := shape["items"].(map[string]any)
	if !ok || items["type"] != "object" {
		t.Fatalf("result_shape items = %#v", shape["items"])
	}
	fields, ok := items["fields"].([]any)
	if !ok {
		t.Fatalf("result_shape fields = %#v", items["fields"])
	}
	want := []string{"drive_asset_id", "content_type", "bytes", "public_url", "reference_asset_ids"}
	for i, field := range want {
		if i >= len(fields) || fields[i] != field {
			t.Fatalf("result_shape fields = %#v, want %#v", fields, want)
		}
	}
}

func containsAnyString(values []any, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
