import { AppIcon } from "@/components/icon"

type DocsMediaPlaceholderProps = {
  type: "image" | "video"
  title: string
  description: string
  className?: string
  bleed?: boolean
}

export function DocsMediaPlaceholder({
  type,
  title,
  description,
  className = "",
  bleed,
}: DocsMediaPlaceholderProps) {
  const shouldBleed = bleed ?? type === "image"

  return (
    <figure
      className={`${
        shouldBleed
          ? "relative left-1/2 w-full -translate-x-1/2 lg:w-[calc(100%+6rem)] 2xl:w-[calc(100%+10rem)]"
          : "w-full"
      } ${className}`}
    >
      <div className="flex aspect-video min-h-64 items-center justify-center rounded-xl border border-dashed border-border bg-surface-secondary px-6 py-12 text-center">
        <div className="max-w-lg">
          <span className="mx-auto flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-surface text-muted">
            <AppIcon
              icon={type === "video" ? "video" : "image"}
              className="h-4.5 w-4.5"
            />
          </span>
          <p className="mt-5 text-xs font-semibold tracking-[0.12em] text-muted uppercase">
            {type === "video" ? "Video placeholder" : "Image placeholder"}
          </p>
          <figcaption className="mt-2 text-base font-semibold text-foreground">
            {title}
          </figcaption>
          <p className="mt-2 text-sm leading-6 text-muted">{description}</p>
        </div>
      </div>
    </figure>
  )
}
