"use client"

import { useState } from "react"
import { Lightbox } from "@/app/w/(chat)/_components/lightbox"
import type { MediaAttachment } from "@/app/w/(chat)/_lib/static-data"

type DocsScreenshotProps = {
  title: string
  lightSrc: string
  darkSrc: string
}

export function DocsScreenshot({
  title,
  lightSrc,
  darkSrc,
}: DocsScreenshotProps) {
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null)
  const screenshots: MediaAttachment[] = [
    {
      id: `${title}-light`,
      filename: `${title} (light theme)`,
      kind: "image",
      url: lightSrc,
    },
    {
      id: `${title}-dark`,
      filename: `${title} (dark theme)`,
      kind: "image",
      url: darkSrc,
    },
  ]

  const openScreenshot = () => {
    setLightboxIndex(
      document.documentElement.classList.contains("dark") ? 1 : 0
    )
  }

  return (
    <>
      <button
        type="button"
        onClick={openScreenshot}
        className="group block w-full cursor-zoom-in overflow-hidden rounded-xl border border-border bg-surface-secondary text-left shadow-sm transition-shadow outline-none hover:shadow-md focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2 focus-visible:ring-offset-background"
        aria-label={`Open ${title} in a lightbox`}
      >
        {/* Theme-specific screenshots follow Hivy's explicit theme setting, not only the OS preference. */}
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={lightSrc}
          alt={title}
          loading="lazy"
          decoding="async"
          className="block h-auto w-full dark:hidden"
        />
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={darkSrc}
          alt=""
          aria-hidden="true"
          loading="lazy"
          decoding="async"
          className="hidden h-auto w-full dark:block"
        />
      </button>
      <Lightbox
        items={screenshots}
        index={lightboxIndex}
        onIndexChange={setLightboxIndex}
        onClose={() => setLightboxIndex(null)}
      />
    </>
  )
}
