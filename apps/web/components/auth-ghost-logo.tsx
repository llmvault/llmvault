"use client"

import { motion } from "motion/react"

import { LogoMark } from "@/components/logo"
import { cn } from "@/lib/utils"

interface AuthGhostLogoProps {
  className?: string
  logoClassName?: string
  title?: string
  description?: string
}

export function AuthGhostLogo({
  className,
  logoClassName,
  title,
  description,
}: AuthGhostLogoProps) {
  return (
    <div className={cn("flex flex-col items-center gap-4", className)}>
      <motion.div
        animate={{ scale: [1, 1.18, 1] }}
        transition={{ duration: 1.2, repeat: Infinity, ease: "easeInOut" }}
      >
        <LogoMark
          className={cn(
            "mx-auto h-12 w-12 drop-shadow-[0_0_16px_rgba(0,174,255,0.28)]",
            logoClassName
          )}
        />
      </motion.div>

      {(title || description) && (
        <div className="space-y-1 text-center">
          {title ? (
            <p className="text-sm font-medium text-foreground">{title}</p>
          ) : null}
          {description ? (
            <p className="text-sm text-muted-foreground">{description}</p>
          ) : null}
        </div>
      )}
    </div>
  )
}
