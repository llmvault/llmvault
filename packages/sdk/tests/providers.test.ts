import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("providers", () => {
  it("lists providers", async () => {
    const { data, error } = await vault.providers.list();

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(Array.isArray(data)).toBe(true);
    expect(data!.length).toBeGreaterThan(0);
    expect(data![0].id).toBeDefined();
    expect(data![0].name).toBeDefined();
  });

  it("gets a single provider", async () => {
    const { data: providers } = await vault.providers.list();
    const first = providers![0];

    const { data, error } = await vault.providers.get(first.id!);

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.id).toBe(first.id);
    expect(data!.models).toBeDefined();
  });

  it("lists models for a provider", async () => {
    const { data: providers } = await vault.providers.list();
    const first = providers![0];

    const { data, error } = await vault.providers.listModels(first.id!);

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(Array.isArray(data)).toBe(true);
  });
});
