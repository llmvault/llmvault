import type createClient from "openapi-fetch";
import type { paths } from "../generated/schema.js";

export type ApiClient = ReturnType<typeof createClient<paths>>;

export class BaseResource {
  constructor(protected client: ApiClient) {}
}
