import { cn } from "@/lib/utils"

export function ModelNewBadge({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "shrink-0 rounded-full bg-success/15 px-1.5 py-0.5 text-[10px] leading-none font-semibold text-success",
        className
      )}
    >
      New
    </span>
  )
}
