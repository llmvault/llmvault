package registry

type HivyModel struct {
	ID     string
	Routes []ModelRoute
	// ProxyRoutes is the ordered, OpenAI-compatible provider chain used by
	// the LLM proxy. A nil value uses Routes in their declared order.
	ProxyRoutes []ModelRoute
}

var hivyModelsByID = func() map[string]HivyModel {
	out := make(map[string]HivyModel, len(supportedHivyModels))
	for _, model := range supportedHivyModels {
		out[model.ID] = model
	}
	return out
}()
