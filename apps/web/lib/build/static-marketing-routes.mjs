import { readFile } from "node:fs/promises"
import { pathToFileURL } from "node:url"

export const staticMarketingRoutes = [
  "/",
  "/access-control",
  "/agents",
  "/automations",
  "/blog",
  "/docs",
  "/drive",
  "/home/variant-1",
  "/home/variant-2",
  "/home/variant-3",
  "/home/variant-4",
  "/home/variant-5",
  "/home/variant-6",
  "/knowledge",
  "/pricing",
  "/sheets",
  "/tag",
]

export const runtimeMarketingRoutes = ["/models"]
export const retiredMarketingRoutes = ["/home"]

const staticMarketingCollections = [
  { route: "/blog/[slug]", prefix: "/blog/" },
  { route: "/docs/[...slug]", prefix: "/docs/" },
]

export function assertStaticMarketingRoutes(manifest) {
  const prerenderedRoutes = new Set(Object.keys(manifest.routes ?? {}))
  const missingRoutes = staticMarketingRoutes.filter(
    (route) => !prerenderedRoutes.has(route)
  )
  const restoredRoutes = retiredMarketingRoutes.filter((route) =>
    prerenderedRoutes.has(route)
  )
  const prerenderedRuntimeRoutes = runtimeMarketingRoutes.filter((route) =>
    prerenderedRoutes.has(route)
  )

  for (const collection of staticMarketingCollections) {
    const generatedCount = [...prerenderedRoutes].filter(
      (route) =>
        route !== collection.prefix.slice(0, -1) &&
        route.startsWith(collection.prefix)
    ).length
    const dynamicRoute = manifest.dynamicRoutes?.[collection.route]

    if (
      !dynamicRoute ||
      dynamicRoute.fallback !== null ||
      generatedCount === 0
    ) {
      missingRoutes.push(collection.route)
    }
  }

  if (missingRoutes.length > 0) {
    throw new Error(
      `Marketing routes must be prerendered: ${missingRoutes.join(", ")}`
    )
  }
  if (restoredRoutes.length > 0) {
    throw new Error(
      `Retired marketing routes must stay removed: ${restoredRoutes.join(", ")}`
    )
  }
  if (prerenderedRuntimeRoutes.length > 0) {
    throw new Error(
      `Runtime marketing routes must stay dynamic: ${prerenderedRuntimeRoutes.join(", ")}`
    )
  }
}

async function checkBuildManifest() {
  const manifestPath = new URL(
    "../../.next/prerender-manifest.json",
    import.meta.url
  )
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"))
  assertStaticMarketingRoutes(manifest)
  process.stdout.write("✓ Marketing routes are statically prerendered.\n")
}

const entryPoint = process.argv[1]
if (entryPoint && pathToFileURL(entryPoint).href === import.meta.url) {
  await checkBuildManifest()
}
