package registry

var coreHivyModels = append(
	append(
		append(
			append(
				append(
					append([]HivyModel{}, coreHivyModelsPreferred...),
					coreHivyModelsText...,
				),
				quantisedHivyModels...,
			),
			engyHivyModels...,
		),
		theseanHivyModels...,
	),
	append(togetherHivyModels, theGridHivyModels...)...,
)
