import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("usage", () => {
  it("gets usage stats", async () => {
    const { data, error } = await vault.usage.get();

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.credentials).toBeDefined();
    expect(data!.tokens).toBeDefined();
    expect(data!.api_keys).toBeDefined();
    expect(data!.requests).toBeDefined();
  });
});
