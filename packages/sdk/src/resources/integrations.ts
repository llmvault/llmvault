import { BaseResource } from "./base.js";
import type {
  CreateIntegrationRequest,
  UpdateIntegrationRequest,
} from "../types.js";

export class IntegrationsResource extends BaseResource {
  create(body: CreateIntegrationRequest) {
    return this.client.POST("/v1/integrations", { body });
  }

  list(query?: {
    limit?: number;
    cursor?: string;
    provider?: string;
    meta?: string;
  }) {
    return this.client.GET("/v1/integrations", { params: { query } });
  }

  get(id: string) {
    return this.client.GET("/v1/integrations/{id}", {
      params: { path: { id } },
    });
  }

  update(id: string, body: UpdateIntegrationRequest) {
    return this.client.PUT("/v1/integrations/{id}", {
      params: { path: { id } },
      body,
    });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/integrations/{id}", {
      params: { path: { id } },
    });
  }

  listProviders() {
    return this.client.GET("/v1/integrations/providers");
  }
}
