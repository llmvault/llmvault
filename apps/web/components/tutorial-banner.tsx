"use client"

import { useState } from "react"
import { Button, Modal } from "@heroui/react"
import NextLink from "next/link"
import { AppIcon } from "@/components/icon"
import { TutorialVideo, youtubeEmbedURL } from "@/components/tutorial-video"
import { clientConfig } from "@/lib/config/public-config"

export function TutorialBanner({
  tutorial,
  title,
  description,
  duration = "1 minute",
  docsPath,
}: {
  tutorial: string
  title: string
  description: string
  duration?: string
  docsPath: string
}) {
  const [open, setOpen] = useState(false)
  const config = clientConfig()
  const videoURL = config.tutorialVideos[tutorial] ?? ""
  const hasVideo = Boolean(youtubeEmbedURL(videoURL))
  const docsURL = config.docsUrl
    ? `${config.docsUrl}/${docsPath.replace(/^\//, "")}`
    : ""

  return (
    <>
      <aside className="flex flex-col gap-4 rounded-xl border border-border bg-surface-secondary px-4 py-4 sm:flex-row sm:items-center">
        <div className="flex min-w-0 flex-1 items-start gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default text-foreground">
            <AppIcon icon="video" className="h-4.5 w-4.5" />
          </div>
          <div className="min-w-0">
            <p className="text-sm font-medium text-foreground">{title}</p>
            <p className="mt-0.5 max-w-xl text-sm leading-5 text-muted">
              {description}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2 pl-12 sm:pl-0">
          {docsURL ? (
            <NextLink
              href={docsURL}
              target="_blank"
              rel="noreferrer"
              className="inline-flex h-8 items-center gap-1.5 rounded-lg px-3 text-sm font-medium text-foreground transition-colors hover:bg-default focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
            >
              Read guide
              <AppIcon icon="external-link" className="h-3.5 w-3.5" />
            </NextLink>
          ) : null}
          {hasVideo ? (
            <Button size="sm" variant="primary" onPress={() => setOpen(true)}>
              <AppIcon icon="play" className="h-3.5 w-3.5" />
              Watch {duration}
            </Button>
          ) : null}
        </div>
      </aside>

      <Modal isOpen={open} onOpenChange={setOpen}>
        <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
          <Modal.Container placement="center" className="p-4">
            <Modal.Dialog className="w-full max-w-4xl border border-border bg-background p-4 shadow-xl outline-none">
              <Modal.CloseTrigger />
              <Modal.Header className="px-2 pb-3">
                <Modal.Heading>{title}</Modal.Heading>
              </Modal.Header>
              <Modal.Body>
                <TutorialVideo
                  url={videoURL}
                  title={`${title} video tutorial`}
                  autoplay
                />
              </Modal.Body>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </>
  )
}
