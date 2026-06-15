import { AuthProvider } from "@/lib/auth/auth-context"
import { WorkspaceProvisioningGate } from "./_components/provisioning-gate"
import { WorkspaceShell } from "./_components/shell"

export default function WorkspaceLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider>
      <WorkspaceProvisioningGate>
        <WorkspaceShell>{children}</WorkspaceShell>
      </WorkspaceProvisioningGate>
    </AuthProvider>
  )
}
