import { BaseResource } from "./base.js";

export class ProvidersResource extends BaseResource {
  list() {
    return this.client.GET("/v1/providers");
  }

  get(id: string) {
    return this.client.GET("/v1/providers/{id}", {
      params: { path: { id } },
    });
  }

  listModels(id: string) {
    return this.client.GET("/v1/providers/{id}/models", {
      params: { path: { id } },
    });
  }
}
