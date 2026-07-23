package registry

var coreHivyModels = append(
	append(
		append(
			append([]HivyModel{}, coreHivyModelsPreferred...),
			coreHivyModelsOpenRouter...,
		),
		quantisedHivyModels...,
	),
	engyHivyModels...,
)
