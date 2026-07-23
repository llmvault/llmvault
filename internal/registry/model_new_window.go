package registry

import "time"

const modelReleaseDateLayout = "2006-01-02"

func (r *Registry) modelWithNewWindow(model Model, hivyModel HivyModel) Model {
	releaseDate := model.ReleaseDate
	if _, err := time.Parse(modelReleaseDateLayout, releaseDate); err != nil {
		releaseDate = r.canonicalReleaseDate(hivyModel)
		if releaseDate != "" {
			model.ReleaseDate = releaseDate
		}
	}
	from, to := newWindow(hivyModel, releaseDate)
	model.NewFrom = from
	model.NewTo = to
	return model
}

func (r *Registry) canonicalReleaseDate(hivyModel HivyModel) string {
	for _, route := range catalogRoutes(hivyModel) {
		model, ok := r.providerModel(route.ProviderID, route.ModelID)
		if !ok {
			continue
		}
		if _, err := time.Parse(modelReleaseDateLayout, model.ReleaseDate); err == nil {
			return model.ReleaseDate
		}
	}
	return ""
}

func newWindow(hivyModel HivyModel, releaseDate string) (*time.Time, *time.Time) {
	if hivyModel.NewFrom != "" || hivyModel.NewTo != "" {
		from, fromErr := time.Parse(time.RFC3339, hivyModel.NewFrom)
		to, toErr := time.Parse(time.RFC3339, hivyModel.NewTo)
		if fromErr != nil || toErr != nil || !from.Before(to) {
			return nil, nil
		}
		return &from, &to
	}

	from, err := time.Parse(modelReleaseDateLayout, releaseDate)
	if err != nil {
		return nil, nil
	}
	to := from.AddDate(0, 2, 0)
	return &from, &to
}
