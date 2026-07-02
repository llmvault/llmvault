"use client"

import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useEffect, useState } from "react"

// TODO: replace with the real demo video ID (the part after `v=` in a YouTube URL)
const DEMO_VIDEO_ID = "dQw4w9WgXcQ"

export function WatchDemo() {
  const [open, setOpen] = useState(false)

  // Close on Escape and lock background scroll while the modal is open.
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false)
    }
    document.addEventListener("keydown", onKey)
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.removeEventListener("keydown", onKey)
      document.body.style.overflow = prevOverflow
    }
  }, [open])

  return (
    <>
      <Button size="lg" variant="secondary" onPress={() => setOpen(true)}>
        <AppIcon icon="play" className="h-5 w-5" />
        Watch the 2min demo
      </Button>

      {open && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label="Hivy demo video"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm sm:p-8"
          onClick={() => setOpen(false)}
        >
          <button
            type="button"
            aria-label="Close demo"
            onClick={() => setOpen(false)}
            className="absolute top-4 right-4 flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white transition-colors hover:bg-white/20 sm:top-6 sm:right-6"
          >
            <AppIcon icon="x" className="h-6 w-6" />
          </button>

          <div
            className="aspect-video w-full max-w-5xl overflow-hidden rounded-2xl bg-black shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <iframe
              src={`https://www.youtube.com/embed/${DEMO_VIDEO_ID}?autoplay=1&rel=0`}
              title="Hivy demo"
              allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
              allowFullScreen
              className="h-full w-full"
            />
          </div>
        </div>
      )}
    </>
  )
}
