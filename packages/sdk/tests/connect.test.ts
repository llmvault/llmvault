import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("connect", () => {
  describe("sessions", () => {
    it("creates a connect session", async () => {
      const { data, error } = await vault.connect.sessions.create({
        external_id: `sdk-test-${Date.now()}`,
        permissions: ["create", "list"],
        ttl: "5m",
      });

      expect(error).toBeUndefined();
      expect(data).toBeDefined();
      expect(data!.id).toBeDefined();
      expect(data!.session_token).toBeDefined();
      expect(data!.session_token!.startsWith("csess_")).toBe(true);
    });
  });

  describe("settings", () => {
    it("gets connect settings", async () => {
      const { data, error } = await vault.connect.settings.get();

      expect(error).toBeUndefined();
      expect(data).toBeDefined();
      expect(Array.isArray(data!.allowed_origins)).toBe(true);
    });

    it("updates connect settings", async () => {
      // Get current, update, then restore
      const { data: current } = await vault.connect.settings.get();
      const original = current!.allowed_origins ?? [];

      const { data, error } = await vault.connect.settings.update({
        allowed_origins: [...original, "https://sdk-test.example.com"],
      });

      expect(error).toBeUndefined();
      expect(data).toBeDefined();
      expect(data!.allowed_origins).toContain("https://sdk-test.example.com");

      // Restore
      await vault.connect.settings.update({
        allowed_origins: original,
      });
    });
  });
});
