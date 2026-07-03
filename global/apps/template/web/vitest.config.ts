import { defineConfig } from "vitest/config"

// Vitest config kept separate from vite.config.ts so the build toolchain and
// the test toolchain stay independent. The realtime engine talks to an
// injected EventSource stub and a real QueryClient, so a plain node
// environment is enough — no DOM needed.
export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
})
