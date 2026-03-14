import { BaseResource } from "./base.js";

export class UsageResource extends BaseResource {
  get() {
    return this.client.GET("/v1/usage");
  }
}
