import { notFound } from "next/navigation"
import { AdminClient } from "./admin-client"

export default function AdminPage() {
  if (process.env.NEXT_PUBLIC_HIVY_ADMIN_ENABLED !== "true") {
    notFound()
  }

  return <AdminClient />
}
