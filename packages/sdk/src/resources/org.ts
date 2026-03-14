import { BaseResource } from "./base.js";

export class OrgResource extends BaseResource {
  getCurrent() {
    return this.client.GET("/v1/orgs/current");
  }
}
