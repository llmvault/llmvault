import { BaseResource } from "./base.js";
import type { CreateAPIKeyRequest } from "../types.js";

export class ApiKeysResource extends BaseResource {
  create(body: CreateAPIKeyRequest) {
    return this.client.POST("/v1/api-keys", { body });
  }

  list(query?: { limit?: number; cursor?: string }) {
    return this.client.GET("/v1/api-keys", { params: { query } });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/api-keys/{id}", {
      params: { path: { id } },
    });
  }
}
