import { notFound } from "next/navigation"
import { AdminClient } from "./admin-client"

// Admin routes are gated by a server-side runtime flag. force-dynamic so the
// value is read per-request (not baked at build time) and the page is never
// statically prerendered.
export const dynamic = "force-dynamic"

export default function AdminPage() {
  if (process.env.HIVY_ADMIN_ENABLED !== "true") {
    notFound()
  }

  return <AdminClient />
}
