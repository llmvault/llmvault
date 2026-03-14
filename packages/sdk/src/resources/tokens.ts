import { BaseResource } from "./base.js";
import type { MintTokenRequest } from "../types.js";

export class TokensResource extends BaseResource {
  create(body: MintTokenRequest) {
    return this.client.POST("/v1/tokens", { body });
  }

  delete(jti: string) {
    return this.client.DELETE("/v1/tokens/{jti}", {
      params: { path: { jti } },
    });
  }
}
