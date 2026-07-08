"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useAuth } from "@/lib/auth/auth-context"
import { useIsAdmin } from "@/lib/auth/use-role"

export default function SettingsIndexPage() {
  const router = useRouter()
  const isAdmin = useIsAdmin()
  const { isLoading } = useAuth()

  useEffect(() => {
    if (isLoading) return
    router.replace(isAdmin ? "/w/settings/general" : "/w/settings/teams")
  }, [isAdmin, isLoading, router])

  return null
}
