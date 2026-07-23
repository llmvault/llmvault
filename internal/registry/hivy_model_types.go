package registry

type HivyModel struct {
	ID     string
	Routes []ModelRoute
	// NewFrom and NewTo override the two-month badge window derived from the
	// canonical model's release date. Both values must be RFC 3339 timestamps.
	NewFrom string
	NewTo   string
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
