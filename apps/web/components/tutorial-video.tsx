import { cn } from "@/lib/utils"

export function youtubeEmbedURL(raw: string | null | undefined): string | null {
  const value = raw?.trim()
  if (!value) return null

  try {
    const url = new URL(value)
    let videoID = ""
    if (url.hostname === "youtu.be") {
      videoID = url.pathname.slice(1).split("/")[0] ?? ""
    } else if (
      url.hostname === "youtube.com" ||
      url.hostname === "www.youtube.com" ||
      url.hostname === "m.youtube.com"
    ) {
      if (url.pathname === "/watch") videoID = url.searchParams.get("v") ?? ""
      else if (url.pathname.startsWith("/embed/"))
        videoID = url.pathname.split("/")[2] ?? ""
      else if (url.pathname.startsWith("/shorts/"))
        videoID = url.pathname.split("/")[2] ?? ""
    }
    if (!/^[A-Za-z0-9_-]{6,20}$/.test(videoID)) return null
    return `https://www.youtube-nocookie.com/embed/${videoID}?rel=0`
  } catch {
    return null
  }
}

export function TutorialVideo({
  url,
  title,
  autoplay = false,
  className,
}: {
  url: string
  title: string
  autoplay?: boolean
  className?: string
}) {
  const embedURL = youtubeEmbedURL(url)
  if (!embedURL) return null

  return (
    <div
      className={cn(
        "aspect-video w-full overflow-hidden rounded-xl bg-default",
        className
      )}
    >
      <iframe
        src={`${embedURL}${autoplay ? "&autoplay=1" : ""}`}
        title={title}
        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
        allowFullScreen
        className="h-full w-full"
      />
    </div>
  )
}
