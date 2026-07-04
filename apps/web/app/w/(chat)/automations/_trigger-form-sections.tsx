import { Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"

export function FormSection({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold text-foreground">{title}</h2>
        <p className="text-muted-foreground text-sm leading-5">{description}</p>
      </div>
      {children}
    </section>
  )
}

export function FieldSkeleton() {
  return <Skeleton className="h-9 rounded-md" />
}

export function InlineNotice({
  icon,
  title,
  body,
}: {
  icon: string
  title: string
  body: string
}) {
  return (
    <div className="flex gap-3 rounded-xl border border-border p-3">
      <div className="text-muted-foreground flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
        <AppIcon icon={icon} className="h-4 w-4" />
      </div>
      <div>
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="text-muted-foreground mt-0.5 text-sm leading-5">{body}</p>
      </div>
    </div>
  )
}
