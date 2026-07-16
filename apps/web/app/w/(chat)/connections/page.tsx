"use client"

import { ConnectionsStep } from "@/app/w/onboarding/connections-step"

export default function ConnectionsPage() {
  return (
    <div className="h-full overflow-y-auto bg-background px-6 py-12 text-foreground">
      <ConnectionsStep showContinue={false} />
    </div>
  )
}
