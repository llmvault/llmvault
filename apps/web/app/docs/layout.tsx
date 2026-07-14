import type { Metadata } from "next"
import { DocsShell } from "./_components/docs-shell"

export const metadata: Metadata = {
  title: {
    default: "Documentation",
    template: "%s | Hivy Documentation",
  },
  description:
    "Learn how to organize teams, work with agents, connect tools, manage knowledge, and automate work with Hivy.",
}

export default function DocsLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return <DocsShell>{children}</DocsShell>
}
