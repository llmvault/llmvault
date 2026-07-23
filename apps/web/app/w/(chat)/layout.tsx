import { AuthProvider } from "@/lib/auth/auth-context"
import { OnboardingGate } from "@/lib/auth/onboarding-gate"
import { WorkspaceShell } from "@/app/w/(chat)/_components/shell"

export default function WorkspaceLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider>
      <OnboardingGate>
        <WorkspaceShell>{children}</WorkspaceShell>
      </OnboardingGate>
    </AuthProvider>
  )
}
