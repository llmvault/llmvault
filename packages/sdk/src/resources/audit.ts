import { BaseResource } from "./base.js";

export class AuditResource extends BaseResource {
  list(query?: { limit?: number; cursor?: string; action?: string }) {
    return this.client.GET("/v1/audit", { params: { query } });
  }
}
