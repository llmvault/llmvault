import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("audit", () => {
  it("lists audit entries", async () => {
    const { data, error } = await vault.audit.list({ limit: 5 });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(Array.isArray(data!.data)).toBe(true);
    expect(typeof data!.has_more).toBe("boolean");
  });
});
