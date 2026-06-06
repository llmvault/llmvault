package registry

type HivyModel struct {
	ID     string
	Routes []ModelRoute
}

var hivyModelsByID = func() map[string]HivyModel {
	out := make(map[string]HivyModel, len(supportedHivyModels))
	for _, model := range supportedHivyModels {
		out[model.ID] = model
	}
	return out
}()
