"use client"

import { use } from "react"
import Image from "next/image"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Link, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { integrationLogoURL } from "@/components/integration-logo"
import { cn } from "@/lib/utils"
import {
  type ApiPlugin,
  PLUGINS_QUERY_KEY,
  pluginCanInstall,
  pluginCapabilities,
  pluginDescription,
  pluginDetailCategory,
  pluginDeveloper,
  pluginIcon,
  pluginIconColor,
  pluginLogoProvider,
  pluginLongDescription,
  pluginMissingRequirements,
  pluginName,
} from "@/app/w/(chat)/plugins/_lib"

export default function PluginDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>
}) {
  const { slug } = use(params)
  const queryClient = useQueryClient()
  const pluginQuery = $api.useQuery("get", "/v1/plugins/{slug}", {
    params: { path: { slug } },
  })
  const installPlugin = $api.useMutation("post", "/v1/plugins/{slug}/install")
  const uninstallPlugin = $api.useMutation(
    "delete",
    "/v1/plugins/{slug}/install"
  )
  const plugin = pluginQuery.data as ApiPlugin | undefined
  const busy = installPlugin.isPending || uninstallPlugin.isPending

  function refresh() {
    queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/plugins/{slug}"] })
  }

  function handleInstall() {
    if (!plugin) return
    installPlugin.mutate(
      { params: { path: { slug } } },
      {
        onSuccess: () => {
          toast.success(`${pluginName(plugin)} plugin added`)
          refresh()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not add plugin")),
      }
    )
  }

  function handleUninstall() {
    if (!plugin) return
    uninstallPlugin.mutate(
      { params: { path: { slug } } },
      {
        onSuccess: () => {
          toast.success(`${pluginName(plugin)} plugin removed`)
          refresh()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not remove plugin")),
      }
    )
  }

  if (pluginQuery.isLoading) {
    return <PluginDetailShell content={<DetailSkeleton />} />
  }

  if (!plugin) {
    return (
      <PluginDetailShell
        content={
          <div className="flex min-h-64 flex-col items-center justify-center rounded-xl border border-border bg-card px-6 text-center">
            <Icon icon="lucide:plug-zap" className="h-7 w-7 text-muted" />
            <p className="mt-3 text-sm font-medium text-foreground">
              Plugin not found
            </p>
            <p className="mt-1 text-sm text-muted">
              This plugin may have been removed from the catalog.
            </p>
          </div>
        }
      />
    )
  }

  const examples = plugin.examples ?? []
  const skills = plugin.skills ?? []
  const links = plugin.links ?? {}
  const missing = pluginMissingRequirements(plugin)
  const canInstall = pluginCanInstall(plugin)

  return (
    <PluginDetailShell
      content={
        <div className="flex flex-col gap-10">
          <header className="flex items-start justify-between gap-4">
            <div className="flex items-center gap-4">
              <div
                className={pluginLogoFrameClass(
                  plugin,
                  "flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl"
                )}
                style={pluginLogoFrameStyle(plugin)}
              >
                <PluginLogo
                  plugin={plugin}
                  size={44}
                  iconSize={32}
                  forceIconWhite
                />
              </div>
              <div className="min-w-0">
                <h1 className="text-3xl font-semibold text-foreground">
                  {pluginName(plugin)}
                </h1>
                <p className="truncate text-lg text-muted-foreground">
                  {pluginDescription(plugin)}
                </p>
              </div>
            </div>
            <Button
              className="shrink-0 rounded-full bg-foreground text-background hover:bg-foreground/90"
              isDisabled={busy || (!plugin.installed && !canInstall)}
              onPress={plugin.installed ? handleUninstall : handleInstall}
            >
              {busy ? <Spinner color="current" size="sm" /> : null}
              {plugin.installed ? "Remove" : "Add"}
            </Button>
          </header>

          {missing.length > 0 ? (
            <section className="border-warning/40 bg-warning/10 rounded-xl border p-4">
              <h2 className="text-sm font-medium text-foreground">
                Required connections missing
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Connect {missing.map((item) => item.provider).join(", ")} before
                adding this plugin.
              </p>
            </section>
          ) : null}

          {examples.length > 0 ? (
            <section className="flex flex-col gap-4">
              <h2 className="text-xl font-semibold text-foreground">
                Examples
              </h2>
              <div className="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
                {examples.map((example, index) => (
                  <button
                    key={index}
                    type="button"
                    className="group flex items-center justify-between gap-4 p-4 text-left transition-colors hover:bg-muted/20"
                  >
                    <div className="flex items-center gap-3">
                      <div
                        className={pluginLogoFrameClass(
                          plugin,
                          "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
                        )}
                        style={pluginLogoFrameStyle(plugin)}
                      >
                        <PluginLogo
                          plugin={plugin}
                          size={24}
                          iconSize={16}
                          forceIconWhite
                        />
                      </div>
                      <span className="text-sm text-foreground">{example}</span>
                    </div>
                    <Icon
                      icon="lucide:arrow-right"
                      className="h-4 w-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground"
                    />
                  </button>
                ))}
              </div>
            </section>
          ) : null}

          {skills.length > 0 ? <ContainsSection skills={skills} /> : null}

          <section className="flex flex-col gap-4">
            <h2 className="text-xl font-semibold text-foreground">
              Developed by {pluginDeveloper(plugin)}
            </h2>
            <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
              <div>
                <p className="text-sm text-muted-foreground">Category</p>
                <p className="text-sm font-medium text-foreground">
                  {pluginDetailCategory(plugin)}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Links</p>
                <div className="flex flex-col items-start gap-1">
                  {links.website ? (
                    <ExternalLink href={links.website} label="Website" />
                  ) : null}
                  {links.privacy ? (
                    <ExternalLink href={links.privacy} label="Privacy Policy" />
                  ) : null}
                  {links.terms ? (
                    <ExternalLink href={links.terms} label="Terms of Service" />
                  ) : null}
                  {!links.website && !links.privacy && !links.terms ? (
                    <span className="text-sm text-muted">
                      No links provided
                    </span>
                  ) : null}
                </div>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Capabilities</p>
                <p className="text-sm font-medium text-foreground">
                  {pluginCapabilities(plugin).join(", ")}
                </p>
              </div>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Description</p>
              <p className="mt-1 text-sm leading-relaxed text-foreground">
                {pluginLongDescription(plugin)}
              </p>
            </div>
          </section>
        </div>
      }
    />
  )
}

function PluginDetailShell({ content }: { content: React.ReactNode }) {
  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">{content}</div>
    </div>
  )
}

function ExternalLink({ href, label }: { href: string; label: string }) {
  return (
    <Link href={href} className="inline-flex items-center gap-1 text-sm">
      {label}
      <Icon icon="lucide:external-link" className="h-3.5 w-3.5" />
    </Link>
  )
}

function ContainsSection({
  skills,
}: {
  skills: Array<{ name?: string; description?: string }>
}) {
  const previewCount = Math.min(5, skills.length)
  const hasMore = skills.length > previewCount

  return (
    <section className="flex flex-col gap-4">
      <div>
        <h2 className="text-xl font-semibold text-foreground">Contains</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Skills {skills.length}
        </p>
      </div>
      <div className="flex flex-col gap-4">
        {skills.slice(0, previewCount).map((skill, index) => (
          <div
            key={skill.name || index}
            className="grid grid-cols-1 gap-1 sm:grid-cols-[1fr_2fr]"
          >
            <span className="text-sm font-medium text-foreground">
              {skill.name || "Skill"}
            </span>
            <span className="text-sm text-muted-foreground">
              {skill.description || "No description available."}
            </span>
          </div>
        ))}
      </div>
      {hasMore ? (
        <button
          type="button"
          className="text-left text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          Show {skills.length - previewCount} more
        </button>
      ) : null}
    </section>
  )
}

function DetailSkeleton() {
  return (
    <div className="flex flex-col gap-10">
      <header className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-4">
          <div className="bg-default h-16 w-16 animate-pulse rounded-2xl" />
          <div className="flex flex-col gap-3">
            <div className="bg-default h-7 w-40 animate-pulse rounded" />
            <div className="bg-default h-5 w-80 max-w-full animate-pulse rounded" />
          </div>
        </div>
        <div className="bg-default h-10 w-20 animate-pulse rounded-full" />
      </header>
      <div className="bg-default h-40 animate-pulse rounded-xl" />
      <div className="bg-default h-56 animate-pulse rounded-xl" />
    </div>
  )
}

function AppIcon({
  icon,
  color,
  size = 20,
}: {
  icon: string
  color: string
  size?: number
}) {
  return (
    <Icon
      icon={icon}
      className="shrink-0"
      style={{ color, width: size, height: size }}
    />
  )
}

function PluginLogo({
  plugin,
  size,
  iconSize = size,
  forceIconWhite = false,
}: {
  plugin: ApiPlugin
  size: number
  iconSize?: number
  forceIconWhite?: boolean
}) {
  const provider = pluginLogoProvider(plugin)
  if (provider) {
    return (
      <Image
        src={integrationLogoURL(provider)}
        alt={provider}
        width={size}
        height={size}
        className="shrink-0 object-contain"
        style={{ width: size, height: size }}
      />
    )
  }
  return (
    <AppIcon
      icon={pluginIcon(plugin)}
      color={forceIconWhite ? "#FFFFFF" : pluginIconColor(plugin)}
      size={iconSize}
    />
  )
}

function pluginLogoFrameClass(plugin: ApiPlugin, className: string): string {
  return cn(className, pluginLogoProvider(plugin) ? "bg-white" : "text-white")
}

function pluginLogoFrameStyle(plugin: ApiPlugin) {
  if (pluginLogoProvider(plugin)) return undefined
  return { backgroundColor: pluginIconColor(plugin) }
}
