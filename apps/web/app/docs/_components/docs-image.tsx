"use client"

import { useState, useSyncExternalStore } from "react"
import Image from "next/image"
import { Modal } from "@heroui/react"
import { motion, useReducedMotion } from "motion/react"
import { useTheme } from "next-themes"
import { AppIcon } from "@/components/icon"

const HIVY_LOGO_ORANGE = "#ff5c00"
const IMAGE_TRANSITION = {
  duration: 0.2,
  ease: [0.16, 1, 0.3, 1] as const,
}

const subscribeToHydration = () => () => undefined

export type DocsImageCallout = {
  arrowHead: string
  label: string
  labelClassName: string
  path: string
}

type DocsImageProps = {
  src: string
  darkSrc?: string
  alt: string
  caption?: string
  callout?: DocsImageCallout | DocsImageCallout[]
  className?: string
  width?: number
  height?: number
  bleed?: boolean
}

export function DocsImage({
  src,
  darkSrc,
  alt,
  caption,
  callout,
  className = "",
  width = 3265,
  height = 2374,
  bleed = true,
}: DocsImageProps) {
  const { resolvedTheme } = useTheme()
  const reduceMotion = useReducedMotion()
  const [open, setOpen] = useState(false)
  const hydrated = useSyncExternalStore(
    subscribeToHydration,
    () => true,
    () => false
  )
  const activeSrc =
    hydrated && resolvedTheme === "dark" && darkSrc ? darkSrc : src
  const transition = reduceMotion ? { duration: 0 } : IMAGE_TRANSITION

  return (
    <>
      <figure
        className={`${
          bleed
            ? "relative left-1/2 w-full -translate-x-1/2 lg:w-[calc(100%+6rem)] 2xl:w-[calc(100%+10rem)]"
            : "w-full"
        } ${className}`}
      >
        <div className="overflow-x-auto rounded-xl border border-border bg-surface-secondary">
          <button
            type="button"
            aria-label={`Open image: ${alt}`}
            onClick={() => setOpen(true)}
            className="group relative block min-w-[720px] cursor-zoom-in overflow-hidden text-left focus-visible:outline-2 focus-visible:outline-offset-[-3px] focus-visible:outline-focus"
          >
            <AnnotatedImage
              src={activeSrc}
              alt={alt}
              width={width}
              height={height}
              callout={callout}
              sizes="(max-width: 768px) 720px, (max-width: 1440px) 900px, 1024px"
            />
            <span className="absolute top-3 right-3 flex h-9 w-9 items-center justify-center rounded-md border border-border bg-overlay/90 text-muted opacity-0 shadow-sm backdrop-blur-sm transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
              <AppIcon icon="maximize-2" className="h-4 w-4" />
            </span>
          </button>
        </div>
        {caption ? (
          <figcaption className="mt-3 text-sm text-muted">{caption}</figcaption>
        ) : null}
      </figure>

      <Modal isOpen={open} onOpenChange={setOpen}>
        <Modal.Backdrop className="bg-background/85 backdrop-blur-lg">
          <Modal.Container
            placement="center"
            className="fixed inset-0 z-50 h-full w-full max-w-none p-0"
          >
            <Modal.Dialog
              aria-label={`Image preview: ${alt}`}
              className="flex h-full w-full max-w-none items-center justify-center rounded-none border-0 bg-transparent p-4 shadow-none outline-none sm:p-8"
            >
              <motion.div
                initial={{ opacity: 0, scale: 0.96, y: 10 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.98, y: 4 }}
                transition={transition}
                className="relative inline-block max-h-[calc(100dvh-4rem)] max-w-[calc(100vw-2rem)] overflow-hidden rounded-xl border border-border bg-surface shadow-2xl sm:max-h-[calc(100dvh-6rem)] sm:max-w-[calc(100vw-4rem)]"
              >
                <AnnotatedImage
                  src={activeSrc}
                  alt={alt}
                  width={width}
                  height={height}
                  callout={callout}
                  sizes="95vw"
                  expanded
                />
              </motion.div>

              <button
                type="button"
                aria-label="Close image preview"
                onClick={() => setOpen(false)}
                className="absolute top-4 right-4 flex h-10 w-10 items-center justify-center rounded-full border border-border bg-overlay/90 text-foreground shadow-md backdrop-blur-sm transition-colors hover:bg-default focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus sm:top-6 sm:right-6"
              >
                <AppIcon icon="x" className="h-4 w-4" />
              </button>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </>
  )
}

function AnnotatedImage({
  src,
  alt,
  width,
  height,
  callout,
  sizes,
  expanded = false,
}: {
  src: string
  alt: string
  width: number
  height: number
  callout?: DocsImageCallout | DocsImageCallout[]
  sizes: string
  expanded?: boolean
}) {
  const callouts = callout ? (Array.isArray(callout) ? callout : [callout]) : []

  return (
    <div className="relative">
      <Image
        src={src}
        alt={alt}
        width={width}
        height={height}
        sizes={sizes}
        className={
          expanded
            ? "block h-auto max-h-[calc(100dvh-4rem)] w-auto max-w-[calc(100vw-2rem)] object-contain sm:max-h-[calc(100dvh-6rem)] sm:max-w-[calc(100vw-4rem)]"
            : "h-auto w-full"
        }
      />
      {callouts.length ? (
        <>
          <svg
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 h-full w-full"
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
          >
            {callouts.map((item) => (
              <g key={`${item.label}-${item.path}`}>
                <path
                  d={item.path}
                  fill="none"
                  stroke={HIVY_LOGO_ORANGE}
                  strokeWidth="3.5"
                  strokeLinecap="butt"
                  vectorEffect="non-scaling-stroke"
                />
                <polygon points={item.arrowHead} fill={HIVY_LOGO_ORANGE} />
              </g>
            ))}
          </svg>
          {callouts.map((item) => (
            <span
              key={item.label}
              className={`absolute rounded-md border border-border bg-overlay/95 px-2.5 py-1.5 text-xs leading-none font-semibold text-foreground shadow-lg ${item.labelClassName}`}
            >
              {item.label}
            </span>
          ))}
        </>
      ) : null}
    </div>
  )
}
