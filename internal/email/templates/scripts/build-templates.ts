import { mkdir, readdir, rm, writeFile } from "node:fs/promises"
import path from "node:path"
import process from "node:process"
import { fileURLToPath } from "node:url"
import type { ComponentType } from "react"
import { createElement } from "react"
import { render } from "@react-email/render"

import { templateRegistry } from "../registry"

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const distDir = path.join(scriptDir, "..", "dist")

type ManifestEntry = {
  alias: string
  subject: string
  variables: string[]
}

async function clean() {
  try {
    const existing = await readdir(distDir)
    await Promise.all(
      existing.map((name) => rm(path.join(distDir, name), { recursive: true, force: true })),
    )
  } catch {
    // dist does not exist yet
  }
  await mkdir(distDir, { recursive: true })
}

async function main() {
  await clean()

  const manifest: ManifestEntry[] = []

  for (const entry of templateRegistry) {
    const alias = entry.definition.alias
    const element = createElement(
      entry.component as ComponentType<Record<string, string>>,
      entry.placeholderProps,
    )

    const html = await render(element, { pretty: true })
    const text = await render(element, { plainText: true })

    await writeFile(path.join(distDir, `${alias}.html`), html, "utf8")
    await writeFile(path.join(distDir, `${alias}.txt`), text, "utf8")

    manifest.push({
      alias,
      subject: entry.definition.subject,
      variables: entry.definition.variables.map((variable) => variable.key),
    })

    console.log(`rendered ${alias} (${manifest[manifest.length - 1].variables.length} variables)`)
  }

  await writeFile(
    path.join(distDir, "manifest.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
    "utf8",
  )

  console.log(`\nWrote ${manifest.length} templates + manifest.json to ${distDir}`)
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
