"use client"

import { use, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Modal, toast, useOverlayState } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import { invalidateSessionListQueries } from "@/app/w/(chat)/_lib/chat-cache"
import {
  type ConnectOptions,
  useConnectIntegration,
} from "@/app/w/(chat)/plugins/use-connect-integration"
import { IntegrationCredentialsForm } from "@/app/w/(chat)/plugins/integration-credentials-form"
import {
  type AvailableIntegration,
  integrationNeedsForm,
} from "@/app/w/(chat)/plugins/integration-auth"
import {
  DatabaseConnectionModalContent,
  isDatabaseProvider,
} from "@/app/w/(chat)/plugins/database-connection-modal-content"
import {
  PluginLogo,
  isDatabaseRequirement,
  isIntegrationRequirement,
  isRequirementMissing,
  pluginLogoFrameClass,
  pluginLogoFrameStyle,
  providerLabel,
} from "@/app/w/(chat)/plugins/[slug]/plugin-detail-helpers"
import {
  ResourceRequirementsSection,
  ResourceSelectionModal,
  type ResourceModalState,
} from "@/app/w/(chat)/plugins/[slug]/resource-requirements-section"
import {
  type ConnectionDisconnectTarget,
  RequiredConnectionsSection,
} from "@/app/w/(chat)/plugins/[slug]/required-connections-section"
import { PluginInstallAction } from "@/app/w/(chat)/plugins/[slug]/plugin-install-action"
import { DisconnectConnectionConfirmDialog } from "@/app/w/(chat)/plugins/[slug]/disconnect-connection-confirm-dialog"
import {
  type ApiPlugin,
  PLUGINS_QUERY_KEY,
  pluginCanInstall,
  pluginDescription,
  pluginMissingRequirements,
  pluginMissingResourceRequirements,
  pluginName,
  pluginShownRequiredConnections,
  type PluginRequirement,
} from "@/app/w/(chat)/plugins/_lib"

type PluginSkill = NonNullable<ApiPlugin["skills"]>[number]
type Connection = components["schemas"]["connectionResponse"]
type DatabaseConnection = components["schemas"]["databaseConnectionResponse"]

type ConnectionModalState = {
  view: "integration" | "database"
  requirement: PluginRequirement
}

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
  const integrationsQuery = $api.useQuery("get", "/v1/integrations/available")
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const databaseConnectionsQuery = $api.useQuery(
    "get",
    "/v1/database-integrations"
  )
  const { connectIntegration, reconnectIntegration, isConnecting } =
    useConnectIntegration()
  const disconnectIntegration = $api.useMutation(
    "delete",
    "/v1/connections/{id}"
  )
  const disconnectDatabase = $api.useMutation(
    "delete",
    "/v1/database-integrations/{id}"
  )
  const [connectionModal, setConnectionModal] =
    useState<ConnectionModalState | null>(null)
  const [resourceModal, setResourceModal] = useState<ResourceModalState | null>(
    null
  )
  const [disconnectTarget, setDisconnectTarget] =
    useState<ConnectionDisconnectTarget | null>(null)
  const connectionModalState = useOverlayState({
    isOpen: connectionModal !== null,
    onOpenChange: (next) => {
      if (!next) setConnectionModal(null)
    },
  })
  const resourceModalState = useOverlayState({
    isOpen: resourceModal !== null,
    onOpenChange: (next) => {
      if (!next) setResourceModal(null)
    },
  })
  const plugin = pluginQuery.data as ApiPlugin | undefined
  const integrations = (integrationsQuery.data ?? []) as AvailableIntegration[]
  const connections = useMemo(
    () => (connectionsQuery.data?.data ?? []) as Connection[],
    [connectionsQuery.data?.data]
  )
  const databaseConnections = useMemo(
    () => (databaseConnectionsQuery.data ?? []) as DatabaseConnection[],
    [databaseConnectionsQuery.data]
  )
  const connectionsByProvider = useMemo(() => {
    const next = new Map<string, Connection>()
    for (const connection of connections) {
      const provider = connection.provider ?? ""
      if (!provider || connection.revoked_at || next.has(provider)) continue
      next.set(provider, connection)
    }
    return next
  }, [connections])
  const databaseConnectionsByProvider = useMemo(() => {
    const next = new Map<string, DatabaseConnection>()
    for (const connection of databaseConnections) {
      const provider = connection.provider ?? ""
      if (!provider || connection.revoked_at || next.has(provider)) continue
      next.set(provider, connection)
    }
    return next
  }, [databaseConnections])
  const disconnectPending =
    disconnectIntegration.isPending || disconnectDatabase.isPending
  const busy =
    installPlugin.isPending ||
    uninstallPlugin.isPending ||
    isConnecting ||
    disconnectPending

  function refresh() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/connections"] })
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/database-integrations"],
    })
    queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/plugins/{slug}"] })
    invalidateSessionListQueries(queryClient)
    pluginQuery.refetch()
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
    if (!plugin || plugin.auto_install === true || plugin.locked === true)
      return
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

  function handleDisconnectRequest(target: ConnectionDisconnectTarget) {
    if (plugin?.installed === true) return
    setDisconnectTarget(target)
  }

  function closeDisconnectDialog(open: boolean) {
    if (!open) setDisconnectTarget(null)
  }

  function handleDisconnectConfirm() {
    if (!disconnectTarget?.connection.id) return

    const mutation =
      disconnectTarget.kind === "database"
        ? disconnectDatabase
        : disconnectIntegration

    mutation.mutate(
      { params: { path: { id: disconnectTarget.connection.id } } },
      {
        onSuccess: () => {
          toast.success(
            `${providerLabel(disconnectTarget.provider)} disconnected`
          )
          setDisconnectTarget(null)
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not disconnect connection")
          ),
      }
    )
  }

  function findIntegration(provider: string): AvailableIntegration | undefined {
    return integrations.find((integration) => integration.provider === provider)
  }

  function closeConnectionModal() {
    setConnectionModal(null)
  }

  function handleConnectRequirement(requirement: PluginRequirement) {
    if (!isRequirementMissing(requirement, plugin ? missing : [])) return

    if (isDatabaseRequirement(requirement)) {
      setConnectionModal({ view: "database", requirement })
      return
    }

    if (!isIntegrationRequirement(requirement)) {
      toast.danger("This plugin needs an unsupported connection")
      return
    }

    const integration = findIntegration(requirement.provider)
    if (!integration?.id) {
      toast.danger(
        `No ${providerLabel(requirement.provider)} integration is available`
      )
      return
    }

    if (integrationNeedsForm(integration)) {
      setConnectionModal({ view: "integration", requirement })
      return
    }

    connectIntegration(integration.id, {
      onSuccess: () => {
        toast.success(`${providerLabel(requirement.provider)} connected`)
        refresh()
      },
    })
  }

  function handleIntegrationConnect(options?: ConnectOptions) {
    const requirement = connectionModal?.requirement
    if (!isIntegrationRequirement(requirement)) return

    const integration = findIntegration(requirement.provider)
    if (!integration?.id) {
      toast.danger(
        `No ${providerLabel(requirement.provider)} integration is available`
      )
      return
    }

    connectIntegration(integration.id, {
      ...options,
      onSuccess: () => {
        closeConnectionModal()
        toast.success(`${providerLabel(requirement.provider)} connected`)
        refresh()
      },
    })
  }

  function handleReconnectRequirement(
    requirement: PluginRequirement,
    connection: Connection
  ) {
    if (!isIntegrationRequirement(requirement) || !connection.id) return
    reconnectIntegration(connection.id, {
      onSuccess: () => {
        toast.success(`${providerLabel(requirement.provider)} reconnected`)
        refresh()
      },
    })
  }

  function handleDatabaseConnected() {
    closeConnectionModal()
    refresh()
  }

  function closeResourceModal() {
    setResourceModal(null)
  }

  function handleResourceSaved() {
    closeResourceModal()
    refresh()
  }

  if (pluginQuery.isLoading) {
    return <PluginDetailShell content={<DetailSkeleton />} />
  }

  if (!plugin) {
    return (
      <PluginDetailShell
        content={
          <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
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
  const missing = pluginMissingRequirements(plugin)
  const missingResources = pluginMissingResourceRequirements(plugin)
  const canInstall = pluginCanInstall(plugin)
  const shownRequiredConnections = pluginShownRequiredConnections(plugin)

  return (
    <>
      <PluginDetailShell
        content={
          <div className="flex flex-col gap-8">
            <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
              <div className="flex min-w-0 items-center gap-3">
                <div
                  className={pluginLogoFrameClass(
                    plugin,
                    "flex h-12 w-12 shrink-0 items-center justify-center rounded-xl"
                  )}
                  style={pluginLogoFrameStyle(plugin)}
                >
                  <PluginLogo
                    plugin={plugin}
                    size={34}
                    iconSize={24}
                    forceIconWhite
                  />
                </div>
                <div className="min-w-0">
                  <h1 className="text-xl font-semibold text-foreground">
                    {pluginName(plugin)}
                  </h1>
                  <p className="text-muted-foreground mt-1 max-w-xl text-sm leading-5">
                    {pluginDescription(plugin)}
                  </p>
                </div>
              </div>
              <PluginInstallAction
                plugin={plugin}
                busy={busy}
                canInstall={canInstall}
                onInstall={handleInstall}
                onUninstall={handleUninstall}
              />
            </header>

            {shownRequiredConnections.length > 0 ? (
              <RequiredConnectionsSection
                requirements={shownRequiredConnections}
                missing={missing}
                integrationsLoading={integrationsQuery.isLoading}
                connectionsByProvider={connectionsByProvider}
                databaseConnectionsByProvider={databaseConnectionsByProvider}
                isBusy={busy}
                disconnectDisabled={plugin.installed === true}
                onConnect={handleConnectRequirement}
                onReconnect={handleReconnectRequirement}
                onDisconnect={handleDisconnectRequest}
              />
            ) : null}

            {missingResources.length > 0 ? (
              <ResourceRequirementsSection
                requirements={missingResources}
                onSelect={(requirement) => setResourceModal({ requirement })}
              />
            ) : null}

            {examples.length > 0 ? (
              <section className="flex flex-col gap-3">
                <h2 className="text-base font-semibold text-foreground">
                  Examples
                </h2>
                <div className="bg-card flex flex-col divide-y divide-border rounded-xl border border-border">
                  {examples.map((example, index) => (
                    <button
                      key={index}
                      type="button"
                      className="group flex items-center justify-between gap-3 px-3 py-2.5 text-left transition-colors hover:bg-muted/20"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        <PluginListLogo plugin={plugin} />
                        <span className="min-w-0 text-sm leading-5 text-foreground">
                          {example}
                        </span>
                      </div>
                      <Icon
                        icon="lucide:arrow-right"
                        className="text-muted-foreground h-4 w-4 shrink-0 transition-colors group-hover:text-foreground"
                      />
                    </button>
                  ))}
                </div>
              </section>
            ) : null}

            {skills.length > 0 ? (
              <SkillsSection plugin={plugin} skills={skills} />
            ) : null}
          </div>
        }
      />

      <RequiredConnectionModal
        modal={connectionModal}
        state={connectionModalState}
        integration={
          connectionModal
            ? findIntegration(connectionModal.requirement.provider ?? "")
            : undefined
        }
        isPending={isConnecting}
        onBack={closeConnectionModal}
        onIntegrationConnect={handleIntegrationConnect}
        onDatabaseConnected={handleDatabaseConnected}
      />
      <ResourceSelectionModal
        modal={resourceModal}
        state={resourceModalState}
        onSaved={handleResourceSaved}
        onCancel={closeResourceModal}
      />
      <DisconnectConnectionConfirmDialog
        target={disconnectTarget}
        pending={disconnectPending}
        onOpenChange={closeDisconnectDialog}
        onConfirm={handleDisconnectConfirm}
      />
    </>
  )
}

function PluginDetailShell({ content }: { content: React.ReactNode }) {
  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">{content}</div>
    </div>
  )
}

function SkillsSection({
  plugin,
  skills,
}: {
  plugin: ApiPlugin
  skills: PluginSkill[]
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold text-foreground">Skills</h2>
      <div className="bg-card flex flex-col divide-y divide-border rounded-xl border border-border">
        {skills.map((skill, index) => (
          <div
            key={skill.name || index}
            className="flex items-start gap-3 px-3 py-2.5"
          >
            <PluginListLogo plugin={plugin} />
            <div className="min-w-0">
              <p className="text-sm leading-5 font-medium text-foreground">
                {skill.name || "Skill"}
              </p>
              <p className="text-muted-foreground text-sm leading-5">
                {skill.human_description ||
                  skill.description ||
                  "No description available."}
              </p>
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

function PluginListLogo({ plugin }: { plugin: ApiPlugin }) {
  return (
    <div
      className={pluginLogoFrameClass(
        plugin,
        "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
      )}
      style={pluginLogoFrameStyle(plugin)}
    >
      <PluginLogo plugin={plugin} size={24} iconSize={16} forceIconWhite />
    </div>
  )
}

function RequiredConnectionModal({
  modal,
  state,
  integration,
  isPending,
  onBack,
  onIntegrationConnect,
  onDatabaseConnected,
}: {
  modal: ConnectionModalState | null
  state: ReturnType<typeof useOverlayState>
  integration: AvailableIntegration | undefined
  isPending: boolean
  onBack: () => void
  onIntegrationConnect: (options?: ConnectOptions) => void
  onDatabaseConnected: () => void
}) {
  const requirement = modal?.requirement
  const provider = requirement?.provider

  return (
    <Modal.Root state={state}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog className="relative w-full max-w-sm rounded-3xl bg-background p-0 shadow-xl outline-none">
            {modal?.view === "database" && isDatabaseProvider(provider) ? (
              <DatabaseConnectionModalContent
                provider={provider}
                onConnected={onDatabaseConnected}
              />
            ) : modal?.view === "integration" && integration ? (
              <IntegrationCredentialsForm
                integration={integration}
                isSubmitting={isPending}
                onBack={onBack}
                onSubmit={onIntegrationConnect}
              />
            ) : null}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal.Root>
  )
}

function DetailSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      <header className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-4">
          <div className="h-12 w-12 animate-pulse rounded-xl bg-default" />
          <div className="flex flex-col gap-3">
            <div className="h-5 w-36 animate-pulse rounded bg-default" />
            <div className="h-4 w-80 max-w-full animate-pulse rounded bg-default" />
          </div>
        </div>
        <div className="h-8 w-16 animate-pulse rounded-full bg-default" />
      </header>
      <div className="h-40 animate-pulse rounded-xl bg-default" />
      <div className="h-56 animate-pulse rounded-xl bg-default" />
    </div>
  )
}
