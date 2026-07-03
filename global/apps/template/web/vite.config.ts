import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"

// The production build lands in ../public, which the Go server serves with
// SPA fallback. `npm run dev` proxies /api and /auth to a locally running
// server binary on :8080.
//
// host: true + allowedHosts let the dev/preview server answer on the
// *.preview.usehivy.com hostnames the platform routes to builder sandboxes.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../public",
    emptyOutDir: true,
  },
  server: {
    host: true,
    allowedHosts: [".preview.usehivy.com"],
    proxy: {
      "/api": "http://localhost:8080",
      "/auth": "http://localhost:8080",
    },
  },
  preview: {
    host: true,
    allowedHosts: [".preview.usehivy.com"],
  },
})
