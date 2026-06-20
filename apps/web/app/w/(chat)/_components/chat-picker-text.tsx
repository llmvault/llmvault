import type { ReactNode } from "react"

export function PickerText({ children }: { children: ReactNode }) {
  return <span className="px-2.5 py-1.5 text-sm text-muted">{children}</span>
}
