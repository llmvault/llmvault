package registry

import "testing"

func TestImageGenerationModelCatalog(t *testing.T) {
	reg := Global()

	reve, ok := reg.ResolveModel("reve", DefaultRasterImageGenerationModelID)
	if !ok {
		t.Fatal("default raster image model route not found")
	}
	if reve.UpstreamID != "reve-image" {
		t.Fatalf("reve upstream = %q", reve.UpstreamID)
	}
	if !ModelSupportsImageOutput(reve.Model) || ModelSupportsTextOutput(reve.Model) {
		t.Fatalf("reve modalities = %#v", reve.Model.Modalities)
	}

	vector, ok := reg.ResolveModel("quiver", DefaultVectorImageGenerationModelID)
	if !ok {
		t.Fatal("default vector image model route not found")
	}
	if vector.UpstreamID != "arrow-1.1" {
		t.Fatalf("vector upstream = %q", vector.UpstreamID)
	}
	if !ModelSupportsVectorImageOutput(vector.Model) || ModelSupportsTextOutput(vector.Model) {
		t.Fatalf("vector modalities = %#v", vector.Model.Modalities)
	}
}

func TestImageGenerationModelsForProviders(t *testing.T) {
	models := Global().ImageGenerationModelsForProviders([]string{"reve", "quiver", "openrouter"})
	want := map[string]bool{
		"reve-image":              false,
		"arrow-1.1":               false,
		"flux.2-klein-4b":         false,
		"riverflow-v2.5-fast":     false,
		"riverflow-v2.5-pro":      false,
		"recraft-v4.1-vector":     false,
		"recraft-v4.1-pro-vector": false,
	}
	for _, model := range models {
		if _, ok := want[model.ID]; ok {
			want[model.ID] = true
		}
		if !ModelSupportsImageOutput(model.Model) {
			t.Fatalf("non-image model returned: %s", model.ID)
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("image model %s not returned", id)
		}
	}
}

func TestValidateImageGenerationModel(t *testing.T) {
	reg := Global()
	if err := reg.ValidateImageGenerationModel(DefaultRasterImageGenerationModelID, false); err != nil {
		t.Fatalf("validate raster default: %v", err)
	}
	if err := reg.ValidateImageGenerationModel(DefaultVectorImageGenerationModelID, true); err != nil {
		t.Fatalf("validate vector default: %v", err)
	}
	if err := reg.ValidateImageGenerationModel(DefaultVectorImageGenerationModelID, false); err == nil {
		t.Fatal("expected vector model to be rejected for raster preference")
	}
	if err := reg.ValidateImageGenerationModel(DefaultRasterImageGenerationModelID, true); err == nil {
		t.Fatal("expected raster model to be rejected for vector preference")
	}
}
