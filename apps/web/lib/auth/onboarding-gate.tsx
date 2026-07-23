"use client"

import { Fragment, useEffect } from "react"
import { usePathname, useRouter } from "next/navigation"
import { useAuth } from "@/lib/auth/auth-context"

export function OnboardingGate({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const pathname = usePathname()
  const { activeOrg, isLoading } = useAuth()
  const step = activeOrg?.onboarding_step
  const needsOnboarding = Boolean(step && step !== "complete")

  useEffect(() => {
    if (!isLoading && needsOnboarding && pathname !== "/w/onboarding") {
      router.replace("/w/onboarding")
    }
  }, [isLoading, needsOnboarding, pathname, router])

  if (isLoading || needsOnboarding) return null
  return <Fragment key={activeOrg?.id}>{children}</Fragment>
}
