import { spawnSync } from "node:child_process"

const mode = process.argv[2]
if (mode !== "dev" && mode !== "build") {
  throw new Error("run-tauri.mjs expects dev or build")
}

process.env.HIVY_DESKTOP_CLOUD_URL ||= "http://localhost:30112"
const result = spawnSync("pnpm", ["exec", "tauri", mode], {
  env: process.env,
  stdio: "inherit",
})
process.exit(result.status ?? 1)
