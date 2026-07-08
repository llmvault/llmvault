"use client"

import * as React from "react"
import { useState } from "react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"

/**
 * Single agent-avatar renderer. Shows the agent's configured avatar image when
 * one is present, falling back to `fallback` (or an icon) both when there is no
 * URL and when the image fails to load. Size and shape are driven entirely by
 * `className` so one component covers every call site — the agents catalog tile,
 * the sidebar channel logo, and the sidebar session dot all delegate here
 * instead of hand-rolling the same img + onError + icon-fallback markup.
 */
export function AgentAvatar({
  avatarURL,
  icon,
  className,
  iconClassName,
  fallback,
  imageFallback,
}: {
  avatarURL?: string | null
  icon: string
  className?: string
  iconClassName?: string
  /** Shown when there is no avatar URL. Defaults to the `icon`. */
  fallback?: React.ReactNode
  /** Shown when an avatar URL was set but the image failed to load.
   *  Defaults to `fallback` (then the icon). Lets the catalog tile keep its
   *  initials on load failure while no-image cases still show the agent icon. */
  imageFallback?: React.ReactNode
}) {
  const [failed, setFailed] = useState(false)
  const hasURL = Boolean(avatarURL)
  const showImage = hasURL && !failed
  const defaultFallback = fallback ?? (
    <AppIcon icon={icon} className={iconClassName} />
  )

  return (
    <span
      className={cn(
        "bg-default text-muted flex shrink-0 items-center justify-center overflow-hidden",
        className
      )}
    >
      {showImage ? (
        // eslint-disable-next-line @next/next/no-img-element -- agent avatars can come from arbitrary workspace-configured URLs
        <img
          src={avatarURL ?? undefined}
          alt=""
          className="h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      ) : hasURL ? (
        (imageFallback ?? defaultFallback)
      ) : (
        defaultFallback
      )}
    </span>
  )
}
