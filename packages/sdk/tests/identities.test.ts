import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("identities", () => {
  let createdId: string;
  const externalId = `sdk-test-${Date.now()}`;

  it("creates an identity", async () => {
    const { data, error } = await vault.identities.create({
      external_id: externalId,
      meta: { source: "sdk-test" },
      ratelimits: [{ name: "default", limit: 100, duration: 60000 }],
    });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.id).toBeDefined();
    expect(data!.external_id).toBe(externalId);

    createdId = data!.id!;
  });

  it("gets an identity by id", async () => {
    const { data, error } = await vault.identities.get(createdId);

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.id).toBe(createdId);
    expect(data!.external_id).toBe(externalId);
  });

  it("lists identities", async () => {
    const { data, error } = await vault.identities.list({ limit: 5 });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(Array.isArray(data!.data)).toBe(true);
  });

  it("updates an identity", async () => {
    const { data, error } = await vault.identities.update(createdId, {
      meta: { source: "sdk-test", updated: true },
    });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.id).toBe(createdId);
  });

  it("deletes the created identity", async () => {
    const { data, error } = await vault.identities.delete(createdId);

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
  });
});
