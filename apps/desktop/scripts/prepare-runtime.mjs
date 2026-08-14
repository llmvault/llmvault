import { copyFileSync, mkdirSync } from "node:fs"
import { spawnSync } from "node:child_process"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const desktopDir = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const runtimeDir = resolve(desktopDir, "../../sandboxes/runtime")
const result = spawnSync(
  "cargo",
  [
    "build",
    "--manifest-path",
    resolve(runtimeDir, "Cargo.toml"),
    "-p",
    "hivy-sandboxes-runtime",
    "--release",
    "--locked",
  ],
  { stdio: "inherit" }
)

if (result.status !== 0) process.exit(result.status ?? 1)

const executable = process.platform === "win32" ? ".exe" : ""
const source = resolve(
  runtimeDir,
  `target/release/hivy-sandboxes-runtime${executable}`
)
const destination = resolve(
  desktopDir,
  "src-tauri/resources/hivy-sandboxes-runtime"
)
mkdirSync(dirname(destination), { recursive: true })
copyFileSync(source, destination)
