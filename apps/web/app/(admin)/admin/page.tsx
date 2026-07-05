import { notFound } from "next/navigation"
import { clientConfig } from "@/lib/config/public-config"
import { AdminClient } from "./admin-client"

export default function AdminPage() {
  if (!clientConfig().adminEnabled) {
    notFound()
  }

  return <AdminClient />
}
