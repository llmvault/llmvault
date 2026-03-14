import { BaseResource } from "./base.js";
import type {
  CreateConnectSessionRequest,
  ConnectSettingsRequest,
} from "../types.js";

class ConnectSessionsResource extends BaseResource {
  create(body: CreateConnectSessionRequest) {
    return this.client.POST("/v1/connect/sessions", { body });
  }
}

class ConnectSettingsResource extends BaseResource {
  get() {
    return this.client.GET("/v1/settings/connect");
  }

  update(body: ConnectSettingsRequest) {
    return this.client.PUT("/v1/settings/connect", { body });
  }
}

export class ConnectResource extends BaseResource {
  public readonly sessions: ConnectSessionsResource;
  public readonly settings: ConnectSettingsResource;

  constructor(client: ConstructorParameters<typeof BaseResource>[0]) {
    super(client);
    this.sessions = new ConnectSessionsResource(client);
    this.settings = new ConnectSettingsResource(client);
  }
}
