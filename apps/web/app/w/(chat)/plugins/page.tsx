"use client"

import { useMemo, useState } from "react"
import Image from "next/image"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import {
  Button,
  Input,
  Popover,
  Modal,
  Spinner,
  Switch,
  toast,
  useOverlayState,
} from "@heroui/react"
import type { UseOverlayStateReturn } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { integrationLogoURL } from "@/components/integration-logo"
import { cn } from "@/lib/utils"
import {
  type ApiPlugin,
  PLUGINS_QUERY_KEY,
  pluginCanInstall,
  pluginCategories,
  pluginCategory,
  pluginDescription,
  pluginIcon,
  pluginIconColor,
  pluginLogoProvider,
  pluginMatchesCategory,
  pluginMatchesQuery,
  pluginName,
  pluginSlug,
  type PluginCategory,
} from "@/app/w/(chat)/plugins/_lib"

export default function PluginsPage() {
  const queryClient = useQueryClient()
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState<PluginCategory>("All")
  const [selectedPlugin, setSelectedPlugin] = useState<ApiPlugin | null>(null)
  const pluginsQuery = $api.useQuery("get", "/v1/plugins")
  const installPlugin = $api.useMutation("post", "/v1/plugins/{slug}/install")
  const uninstallPlugin = $api.useMutation(
    "delete",
    "/v1/plugins/{slug}/install"
  )
  const plugins = useMemo(
    () => (pluginsQuery.data ?? []) as ApiPlugin[],
    [pluginsQuery.data]
  )
  const categories = useMemo(() => pluginCategories(plugins), [plugins])
  const connectedPlugins = useMemo(
    () => plugins.filter((plugin) => plugin.installed),
    [plugins]
  )
  const modalState = useOverlayState({
    isOpen: selectedPlugin !== null,
    onOpenChange: (next) => {
      if (!next) setSelectedPlugin(null)
    },
  })

  const filteredPlugins = useMemo(() => {
    return plugins.filter(
      (plugin) =>
        pluginMatchesCategory(plugin, category) &&
        pluginMatchesQuery(plugin, query)
    )
  }, [category, plugins, query])

  const groupedPlugins = useMemo(() => {
    if (category !== "All") {
      return { [category]: filteredPlugins }
    }

    const groups: Record<string, ApiPlugin[]> = {}
    for (const plugin of filteredPlugins) {
      const section = pluginCategory(plugin)
      if (!groups[section]) groups[section] = []
      groups[section].push(plugin)
    }

    const ordered: Record<string, ApiPlugin[]> = {}
    for (const section of categories.filter(
      (item) => item !== "All" && item !== "Featured"
    )) {
      if (groups[section]) ordered[section] = groups[section]
    }
    for (const section of Object.keys(groups)) {
      if (!ordered[section]) ordered[section] = groups[section]
    }
    return ordered
  }, [categories, category, filteredPlugins])

  const sectionEntries = Object.entries(groupedPlugins)

  function handleAdd(plugin: ApiPlugin) {
    if (plugin.installed) {
      handleUninstall(plugin)
      return
    }
    setSelectedPlugin(plugin)
  }

  function handleConnect() {
    if (!selectedPlugin) return
    const slug = pluginSlug(selectedPlugin)
    if (!slug) return
    installPlugin.mutate(
      { params: { path: { slug } } },
      {
        onSuccess: () => {
          toast.success(`${pluginName(selectedPlugin)} plugin added`)
          queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY })
          modalState.close()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not add plugin")),
      }
    )
  }

  function handleUninstall(plugin: ApiPlugin) {
    const slug = pluginSlug(plugin)
    if (!slug) return
    uninstallPlugin.mutate(
      { params: { path: { slug } } },
      {
        onSuccess: () => {
          toast.success(`${pluginName(plugin)} plugin removed`)
          queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY })
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not remove plugin")),
      }
    )
  }

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          <nav
            aria-label="Plugins and skills"
            className="flex items-center gap-1"
          >
            <NextLink
              href="/w/plugins"
              className="bg-default rounded-lg px-3 py-1.5 text-sm font-medium text-foreground"
            >
              Plugins
            </NextLink>
          </nav>

          <div>
            <h1 className="text-2xl font-semibold text-foreground">Plugins</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Work with Hivy across your favorite tools
            </p>
          </div>

          <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative min-w-0 flex-1">
              <Icon
                icon="lucide:search"
                className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search plugins and skills"
                className="w-full rounded-md bg-card pl-9"
              />
            </div>

            <CategorySelect
              categories={categories}
              value={category}
              onChange={setCategory}
            />
          </div>

          {connectedPlugins.length > 0 ? (
            <section className="flex flex-col gap-3">
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-medium text-foreground">
                  Connected
                </h2>
                <button
                  type="button"
                  className="text-sm text-muted-foreground transition-colors hover:text-foreground"
                >
                  Manage
                </button>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {connectedPlugins.map((plugin) => (
                  <div
                    key={pluginSlug(plugin)}
                    className={cn(
                      "flex h-8 w-8 items-center justify-center rounded-lg transition-colors hover:bg-muted/40",
                      pluginLogoProvider(plugin) ? "bg-white" : "bg-card"
                    )}
                    title={pluginName(plugin)}
                  >
                    <PluginLogo plugin={plugin} size={20} iconSize={14} />
                  </div>
                ))}
              </div>
            </section>
          ) : null}

          {pluginsQuery.isLoading ? (
            <PluginListSkeleton />
          ) : sectionEntries.length === 0 ? (
            <EmptyState query={query} />
          ) : (
            <div className="flex flex-col gap-8">
              {sectionEntries.map(([section, plugins]) => (
                <section key={section} className="flex flex-col gap-3">
                  <h2 className="text-sm font-medium text-foreground">
                    {section}
                  </h2>
                  <div className="flex flex-col bg-card">
                    {plugins.map((plugin) => (
                      <PluginRow
                        key={pluginSlug(plugin)}
                        plugin={plugin}
                        onAdd={() => handleAdd(plugin)}
                        busy={
                          installPlugin.isPending || uninstallPlugin.isPending
                        }
                      />
                    ))}
                  </div>
                </section>
              ))}
            </div>
          )}
        </div>
      </div>

      <ConnectModal
        plugin={selectedPlugin}
        state={modalState}
        onConnect={handleConnect}
        isPending={installPlugin.isPending}
      />
    </div>
  )
}

function CategorySelect({
  categories,
  value,
  onChange,
}: {
  categories: PluginCategory[]
  value: PluginCategory
  onChange: (category: PluginCategory) => void
}) {
  const [open, setOpen] = useState(false)

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`Category: ${value}`}
        className="flex h-10 w-full items-center justify-between rounded-md bg-card px-3 text-sm text-foreground transition-colors hover:bg-muted/20 sm:w-48"
      >
        <span>{value}</span>
        <Icon
          icon="lucide:chevron-down"
          className="h-4 w-4 text-muted-foreground"
        />
      </Popover.Trigger>
      <Popover.Content className="w-48 rounded-xl p-1.5">
        <Popover.Dialog className="flex max-h-72 w-full flex-col gap-0.5 overflow-y-auto p-0">
          {categories.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => {
                onChange(item)
                setOpen(false)
              }}
              className={cn(
                "flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-sm transition-colors",
                item === value
                  ? "bg-default font-medium text-foreground"
                  : "text-muted-foreground hover:bg-muted/20 hover:text-foreground"
              )}
            >
              <span className="min-w-0 flex-1">{item}</span>
              {item === value ? (
                <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
              ) : null}
            </button>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function PluginRow({
  plugin,
  onAdd,
  busy,
}: {
  plugin: ApiPlugin
  onAdd: () => void
  busy: boolean
}) {
  const canInstall = plugin.installed || pluginCanInstall(plugin)
  return (
    <div className="group -mx-3 py-1.5">
      <div className="rounded-xl px-3 py-1.5 transition-colors group-hover:bg-default">
        <div className="flex items-center gap-3">
          <NextLink
            href={`/w/plugins/${pluginSlug(plugin)}`}
            className="flex min-w-0 flex-1 items-center gap-3"
          >
            <div
              className={pluginLogoFrameClass(
                plugin,
                "flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
              )}
              style={pluginLogoFrameStyle(plugin)}
            >
              <PluginLogo
                plugin={plugin}
                size={27}
                iconSize={18}
                forceIconWhite
              />
            </div>

            <div className="min-w-0 flex-1">
              <h3 className="text-sm font-medium text-foreground">
                {pluginName(plugin)}
              </h3>
              <p className="truncate text-sm text-muted-foreground">
                {pluginDescription(plugin)}
              </p>
            </div>
          </NextLink>

          <Button
            variant="tertiary"
            size="sm"
            className="shrink-0 rounded-full"
            isDisabled={busy || !canInstall}
            onPress={onAdd}
          >
            {busy ? <Spinner color="current" size="sm" /> : null}
            {plugin.installed ? "Remove" : "Add"}
          </Button>
        </div>
      </div>
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

function EmptyState({ query }: { query: string }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <Icon icon="lucide:plug" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query ? "No matching plugins" : "No plugins available"}
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {query
          ? "Try a different search or category."
          : "Browse the catalog to connect your favorite tools."}
      </p>
    </div>
  )
}

function PluginListSkeleton() {
  return (
    <div className="flex flex-col rounded-xl bg-card">
      {[0, 1, 2, 3].map((index) => (
        <div key={index} className="flex items-center gap-3 p-3">
          <div className="bg-default h-8 w-8 shrink-0 animate-pulse rounded-lg" />
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <div className="bg-default h-3.5 w-28 animate-pulse rounded" />
            <div className="bg-default h-3 w-64 max-w-full animate-pulse rounded" />
          </div>
          <div className="bg-default h-8 w-16 shrink-0 animate-pulse rounded-full" />
        </div>
      ))}
    </div>
  )
}

function ConnectModal({
  plugin,
  state,
  onConnect,
  isPending,
}: {
  plugin: ApiPlugin | null
  state: UseOverlayStateReturn
  onConnect: () => void
  isPending: boolean
}) {
  const [referenceMemory, setReferenceMemory] = useState(true)
  const canInstall = plugin ? pluginCanInstall(plugin) : false

  return (
    <Modal.Root state={state}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog className="relative w-full max-w-md rounded-3xl bg-background p-0 shadow-xl outline-none">
            {plugin ? (
              <div className="flex flex-col gap-6 p-6">
                <Button
                  variant="ghost"
                  size="sm"
                  isIconOnly
                  onPress={state.close}
                  aria-label="Close"
                  className="absolute top-4 right-4"
                >
                  <Icon icon="lucide:x" className="h-4 w-4" />
                </Button>

                <div className="flex flex-col items-center gap-4 pt-2 text-center">
                  <div className="flex items-center gap-4">
                    <div
                      className={pluginLogoFrameClass(
                        plugin,
                        "flex h-16 w-16 items-center justify-center rounded-2xl"
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
                    <div className="flex items-center gap-1.5">
                      <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60" />
                      <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60" />
                      <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60" />
                    </div>
                    <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-background">
                      <Icon
                        icon="lucide:bot"
                        className="h-8 w-8 text-foreground"
                      />
                    </div>
                  </div>

                  <div className="flex flex-col items-center gap-2">
                    <h2 className="text-2xl font-semibold text-foreground">
                      Connect {pluginName(plugin)}
                    </h2>
                    <div className="flex items-center gap-1.5 text-success">
                      <span className="flex h-4 w-4 items-center justify-center rounded-full bg-success text-background">
                        <Icon icon="lucide:check" className="h-2.5 w-2.5" />
                      </span>
                      <span className="text-sm font-medium">
                        Approved by your admin
                      </span>
                    </div>
                  </div>
                </div>

                <div className="rounded-2xl bg-card p-5">
                  <div className="flex items-start justify-between gap-4">
                    <div className="text-left">
                      <h3 className="text-base font-medium text-foreground">
                        Reference memories and chats
                      </h3>
                      <p className="mt-1 text-sm text-muted-foreground">
                        Allow Hivy to reference relevant chats and memories when
                        sharing data with {pluginName(plugin)} for more helpful
                        responses.
                      </p>
                    </div>
                    <Switch
                      isSelected={referenceMemory}
                      onChange={setReferenceMemory}
                      className="shrink-0"
                    >
                      <Switch.Control>
                        <Switch.Thumb />
                      </Switch.Control>
                    </Switch>
                  </div>

                  <hr className="my-4 opacity-0" />

                  <div className="text-left">
                    <h3 className="text-base font-medium text-foreground">
                      You&apos;re in control
                    </h3>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Hivy always respects your training data preferences, and
                      is limited to permissions you&apos;ve explicitly set.
                    </p>
                  </div>

                  <hr className="my-4 opacity-0" />

                  <div className="text-left">
                    <h3 className="text-base font-medium text-foreground">
                      Apps may introduce elevated risk
                    </h3>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Hivy is built to protect your data, but attackers may
                      attempt to use Hivy to access your data in the app, or use
                      the app to attempt to access your data in Hivy.
                    </p>
                  </div>

                  <hr className="my-4 opacity-0" />

                  <div className="text-left">
                    <h3 className="text-base font-medium text-foreground">
                      Data shared with this app
                    </h3>
                    <p className="mt-1 text-sm text-muted-foreground">
                      By adding this app, you allow it to access basic
                      information typically shared when you visit a website,
                      such as your IP address and approximate location{" "}
                      <button
                        type="button"
                        className="underline hover:text-foreground"
                      >
                        learn more
                      </button>
                      , and a summary of your recent context and intent within
                      Hivy. Our policies require that apps only access relevant
                      content to respond to your requests. This data will be
                      used as described in the app{" "}
                      <button
                        type="button"
                        className="underline hover:text-foreground"
                      >
                        Terms of Use
                      </button>{" "}
                      and{" "}
                      <button
                        type="button"
                        className="underline hover:text-foreground"
                      >
                        Privacy
                      </button>
                      .
                    </p>
                  </div>
                </div>

                <Button
                  fullWidth
                  className="rounded-full bg-foreground text-background hover:bg-foreground/90"
                  onPress={onConnect}
                  isDisabled={isPending || !canInstall}
                >
                  {isPending ? <Spinner color="current" size="sm" /> : null}
                  Continue to {pluginName(plugin)}
                  <Icon icon="lucide:arrow-up-right" className="ml-1 h-4 w-4" />
                </Button>

                <button
                  type="button"
                  className="text-center text-sm text-muted-foreground underline transition-colors hover:text-foreground"
                >
                  Advanced settings (opens hivy.com)
                </button>
              </div>
            ) : null}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal.Root>
  )
}
