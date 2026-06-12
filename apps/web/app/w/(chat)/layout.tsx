import { WorkspaceShell } from "./_components/shell"

export default function WorkspaceLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return <WorkspaceShell>{children}</WorkspaceShell>
}
