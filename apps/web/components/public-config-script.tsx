import { headers } from "next/headers"
import { PUBLIC_CONFIG_KEY, serializePublicConfig } from "@/lib/config/public-config"

// Injects the runtime public config into the initial HTML as
// window.__HIVY_ENV__, read at REQUEST time (not build time) so a prebuilt image
// can be repointed via env. Awaiting headers() opts this out of static rendering
// so the env is always read at runtime. Rendered as the first element in <body>
// so the value is set before any client component hydrates.
export async function PublicConfigScript() {
  await headers()
  const json = serializePublicConfig()
  return (
    <script
      // eslint-disable-next-line react/no-danger -- inline env bootstrap; value is JSON with < escaped
      dangerouslySetInnerHTML={{ __html: `window.${PUBLIC_CONFIG_KEY} = ${json};` }}
    />
  )
}
