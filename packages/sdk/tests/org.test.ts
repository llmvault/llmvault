import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("org", () => {
  it("gets current organization", async () => {
    const { data, error } = await vault.org.getCurrent();

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.id).toBeDefined();
    expect(data!.name).toBeDefined();
  });
});
