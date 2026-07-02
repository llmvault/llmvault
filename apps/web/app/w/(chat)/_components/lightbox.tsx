"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { AnimatePresence, motion } from "motion/react"
import { Button, Modal } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { MediaAttachment } from "@/app/w/(chat)/_lib/static-data"

const CHROME_TRANSITION = { duration: 0.16, ease: [0.16, 1, 0.3, 1] as const }

export function Lightbox({
  items,
  index,
  onIndexChange,
  onClose,
}: {
  items: MediaAttachment[]
  index: number | null
  onIndexChange: (next: number) => void
  onClose: () => void
}) {
  const open = index !== null
  const current = open ? items[index] : null
  const total = items.length

  // Idle-tracking for the auto-hiding control rail: any pointer activity
  // wakes the chrome; after 2.4s of stillness it fades so the media gets
  // the full stage.
  const [chromeVisible, setChromeVisible] = useState(true)
  const idleTimer = useRef<number | null>(null)
  const wakeChrome = useCallback(() => {
    setChromeVisible(true)
    if (idleTimer.current) window.clearTimeout(idleTimer.current)
    idleTimer.current = window.setTimeout(() => setChromeVisible(false), 2400)
  }, [])

  useEffect(() => {
    if (!open) return
    // Waking the chrome on open/navigation is a timer side effect; the
    // visibility state mirrors the idle timer rather than derived state.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    wakeChrome()
    return () => {
      if (idleTimer.current) window.clearTimeout(idleTimer.current)
    }
  }, [open, index, wakeChrome])

  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "ArrowLeft" && index !== null && index > 0) {
        onIndexChange(index - 1)
      } else if (
        event.key === "ArrowRight" &&
        index !== null &&
        index < total - 1
      ) {
        onIndexChange(index + 1)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [open, index, total, onIndexChange])

  const [copied, setCopied] = useState(false)
  const copyUrl = async () => {
    if (!current) return
    try {
      await navigator.clipboard.writeText(
        new URL(current.url, window.location.origin).toString()
      )
    } catch {
      // Clipboard can be blocked; the checkmark still confirms the action.
    }
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1400)
  }

  return (
    <Modal isOpen={open} onOpenChange={(next) => !next && onClose()}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-xl">
        <Modal.Container
          placement="center"
          className="fixed inset-0 z-50 h-full w-full max-w-none p-0"
        >
          <Modal.Dialog
            aria-label={current?.filename ?? "Media preview"}
            className="flex h-full w-full max-w-none flex-col rounded-none border-0 bg-transparent p-0 shadow-none outline-none"
          >
            {current ? (
              <div
                className="relative flex h-full w-full flex-col"
                onMouseMove={wakeChrome}
                onPointerDown={wakeChrome}
              >
                <AnimatePresence>
                  {chromeVisible ? (
                    <motion.header
                      key="header"
                      initial={{ opacity: 0, y: -6 }}
                      animate={{ opacity: 1, y: 0 }}
                      exit={{ opacity: 0, y: -6 }}
                      transition={CHROME_TRANSITION}
                      className="bg-surface/85 absolute inset-x-0 top-0 z-20 flex items-center gap-3 border-b border-border px-5 py-3 backdrop-blur-md"
                    >
                      <div className="flex min-w-0 flex-1 flex-col">
                        <span className="truncate text-sm font-medium">
                          {current.filename}
                        </span>
                        <span className="text-xs text-muted">
                          {current.kind === "video" ? "Video" : "Image"}
                          {current.duration ? ` · ${current.duration}` : ""}
                        </span>
                      </div>

                      <span className="text-xs text-muted tabular-nums">
                        {(index ?? 0) + 1} / {total}
                      </span>

                      <div className="flex items-center gap-0.5">
                        <Button
                          variant="ghost"
                          size="sm"
                          isIconOnly
                          aria-label="Download"
                          onPress={() => {
                            const link = document.createElement("a")
                            link.href = current.url
                            link.download = current.filename
                            link.click()
                          }}
                        >
                          <AppIcon
                            icon="download"
                            className="h-4 w-4 text-muted"
                          />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          isIconOnly
                          aria-label="Copy URL"
                          onPress={copyUrl}
                        >
                          <AppIcon
                            icon={copied ? "check" : "copy"}
                            className="h-4 w-4 text-muted"
                          />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          isIconOnly
                          aria-label="Close preview"
                          onPress={onClose}
                        >
                          <AppIcon
                            icon="x"
                            className="h-4 w-4 text-muted"
                          />
                        </Button>
                      </div>
                    </motion.header>
                  ) : null}
                </AnimatePresence>

                <div className="relative min-h-0 flex-1 overflow-hidden">
                  {total > 1 ? (
                    <AnimatePresence>
                      {chromeVisible ? (
                        <>
                          <motion.div
                            key="prev"
                            initial={{ opacity: 0, x: -6 }}
                            animate={{ opacity: 1, x: 0 }}
                            exit={{ opacity: 0, x: -6 }}
                            transition={CHROME_TRANSITION}
                            className="absolute top-1/2 left-4 z-10 -translate-y-1/2"
                          >
                            <Button
                              variant="tertiary"
                              size="sm"
                              isIconOnly
                              aria-label="Previous"
                              isDisabled={index === 0}
                              onPress={() => onIndexChange((index ?? 0) - 1)}
                              className="rounded-full shadow-sm"
                            >
                              <AppIcon
                                icon="arrow-left"
                                className="h-4 w-4"
                              />
                            </Button>
                          </motion.div>
                          <motion.div
                            key="next"
                            initial={{ opacity: 0, x: 6 }}
                            animate={{ opacity: 1, x: 0 }}
                            exit={{ opacity: 0, x: 6 }}
                            transition={CHROME_TRANSITION}
                            className="absolute top-1/2 right-4 z-10 -translate-y-1/2"
                          >
                            <Button
                              variant="tertiary"
                              size="sm"
                              isIconOnly
                              aria-label="Next"
                              isDisabled={index === total - 1}
                              onPress={() => onIndexChange((index ?? 0) + 1)}
                              className="rounded-full shadow-sm"
                            >
                              <AppIcon
                                icon="arrow-right"
                                className="h-4 w-4"
                              />
                            </Button>
                          </motion.div>
                        </>
                      ) : null}
                    </AnimatePresence>
                  ) : null}

                  <AnimatePresence mode="wait" initial={false}>
                    <motion.div
                      key={current.id}
                      initial={{ opacity: 0, y: 4 }}
                      animate={{ opacity: 1, y: 0 }}
                      exit={{ opacity: 0, y: -4 }}
                      transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
                      className="absolute inset-0 flex items-center justify-center p-12 pt-20"
                    >
                      {current.kind === "video" ? (
                        <video
                          src={current.url}
                          poster={current.poster}
                          controls
                          autoPlay
                          className="max-h-full max-w-full rounded-2xl border border-border bg-black shadow-lg"
                        />
                      ) : (
                        // eslint-disable-next-line @next/next/no-img-element -- lightbox renders arbitrary sample assets at native size
                        <img
                          src={current.url}
                          alt={current.filename}
                          className="bg-surface max-h-full max-w-full rounded-2xl border border-border object-contain shadow-lg"
                        />
                      )}
                    </motion.div>
                  </AnimatePresence>
                </div>
              </div>
            ) : null}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
