import Image from "next/image"
import { cn } from "@/lib/utils"

export function LogoMark({ className = "h-6 w-6" }: { className?: string }) {
  return (
    <Image
      src="/hivy-logo.png"
      alt=""
      aria-hidden="true"
      width={1024}
      height={1024}
      unoptimized
      className={cn("block object-contain", className)}
    />
  )
}

export function Logo({ className = "h-8" }: { className?: string }) {
  return (
    <div className={`flex items-center gap-2.5 ${className}`}>
      <LogoMark className="h-full w-auto shrink-0" />
      <span
        className="text-lg font-semibold tracking-tight text-foreground"
        style={{ fontFamily: "var(--font-sora), sans-serif" }}
      >
        hivy
      </span>
    </div>
  )
}
