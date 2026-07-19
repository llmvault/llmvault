import { AppIcon } from "@/components/icon"
import { DocsScreenshot } from "./docs-screenshot"

const SCREENSHOTS_BY_TITLE: Record<string, string> = {
  "Members, roles, and pending invitations": "members-roles-invitations",
  "Ricky beside a running app preview": "apps-dashboard-preview",
  "The Apps page grouped by team": "apps-dashboard",
  "A catalog agent’s team installation screen": "agent-catalog-installation",
  "The New chat composer with session choices": "new-session-composer",
  "An automated session and its source": "event-trigger-configuration",
  "The custom agent configuration form": "custom-agent-configuration",
  "Choose a connection and review its requirements":
    "connection-requirements-access",
  "Choose a provider and review existing instances":
    "connection-requirements-access",
  "Set a database access policy": "connection-requirements-access",
  "Enable an installed connection for one team":
    "connection-requirements-access",
  "Connection requirements and access state": "connection-requirements-access",
  "Connection grant and generated tools": "connection-requirements-access",
  "A complete event trigger configuration": "event-trigger-configuration",
  "A session with multiple work views open": "sheet-pages-fields-rows",
  "See where work lives": "workspace-hierarchy",
  "Webhook URL, status, and last run": "webhook-url-status",
  "Source scope and team grants": "knowledge-source-scope",
  "Select a focused source scope": "knowledge-source-scope",
  "Agent memory mission": "agent-memory-mission",
  "Rules and memories for one agent": "agent-memory-mission",
  "Choose DeepSeek V4 Flash": "model-picker-deepseek-reasoning",
  "Review the session and its sheet": "sheet-pages-fields-rows",
  "Schedule fields and cadence preview": "schedule-fields-cadence",
  "A Sheet with pages, typed fields, and rows": "sheet-pages-fields-rows",
  "A linked Slack thread and its Hivy session": "event-trigger-configuration",
  "Team details and resource access": "members-roles-invitations",
  "Tools, sub-agents, and sandbox settings": "agent-tools-subagents",
  "Where a completed session shows its cost": "usage-billing-session-cost",
  "Start a session with the right agent": "new-session-composer",
  "Choose how an automation starts": "automations-overview",
}

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
  const screenshot = type === "image" ? SCREENSHOTS_BY_TITLE[title] : undefined

  if (screenshot) {
    const lightSrc = `/docs/captures/${screenshot}-light.png`
    const darkSrc = `/docs/captures/${screenshot}-dark.png`

    return (
      <figure
        className={`${
          shouldBleed
            ? "relative left-1/2 w-full -translate-x-1/2 lg:w-[calc(100%+6rem)] 2xl:w-[calc(100%+10rem)]"
            : "w-full"
        } ${className}`}
      >
        <DocsScreenshot title={title} lightSrc={lightSrc} darkSrc={darkSrc} />
        <figcaption className="mt-3 px-1 text-sm leading-6 text-muted">
          <span className="font-semibold text-foreground">{title}.</span> Live
          Hivy capture, available in light and dark themes.
        </figcaption>
      </figure>
    )
  }

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
