import { AuthProvider } from "@/lib/auth/auth-context"
import { WorkspaceShell } from "./_components/shell"

export default function WorkspaceLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider>
      <WorkspaceShell>{children}</WorkspaceShell>
    </AuthProvider>
  )
}
