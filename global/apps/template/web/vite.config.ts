import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"

// The production build lands in ../public, which the Go server serves with
// SPA fallback. `npm run dev` proxies /api and /auth to a locally running
// server binary on :8080.
//
// host: true + allowedHosts let the dev/preview server answer on whatever
// preview hostnames the platform routes to builder sandboxes. allowedHosts is
// `true` (accept any host) so the template is domain-agnostic and works under
// any self-hosted preview domain — the sandbox network boundary is the real
// access control here, not the hostname allowlist.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../public",
    emptyOutDir: true,
  },
  server: {
    host: true,
    allowedHosts: true,
    proxy: {
      "/api": "http://localhost:8080",
      "/auth": "http://localhost:8080",
    },
  },
  preview: {
    host: true,
    allowedHosts: true,
  },
})
