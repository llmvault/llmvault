import { headers } from "next/headers"
import {
  PUBLIC_CONFIG_ELEMENT_ID,
  serializePublicConfig,
} from "@/lib/config/public-config"

export async function RuntimePublicConfigLayout({
  children,
}: {
  children: React.ReactNode
}) {
  await headers()
  return (
    <>
      <script
        id={PUBLIC_CONFIG_ELEMENT_ID}
        type="application/json"
        dangerouslySetInnerHTML={{ __html: serializePublicConfig() }}
      />
      {children}
    </>
  )
}
