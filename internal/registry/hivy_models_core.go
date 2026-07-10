package registry

var coreHivyModels = append(
	append([]HivyModel{}, coreHivyModelsPreferred...),
	coreHivyModelsOpenRouter...,
)
