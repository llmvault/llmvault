"use client"

import { use, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Modal, Spinner, toast, useOverlayState } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
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
  RequirementLogo,
  connectionKindLabel,
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
import { PluginInstallAction } from "@/app/w/(chat)/plugins/[slug]/plugin-install-action"
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
  const { connectIntegration, isConnecting } = useConnectIntegration()
  const [connectionModal, setConnectionModal] =
    useState<ConnectionModalState | null>(null)
  const [resourceModal, setResourceModal] = useState<ResourceModalState | null>(
    null
  )
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
  const busy =
    installPlugin.isPending || uninstallPlugin.isPending || isConnecting

  function refresh() {
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
                  <p className="mt-1 max-w-xl text-sm leading-5 text-muted-foreground">
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
                isBusy={busy}
                onConnect={handleConnectRequirement}
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
                <div className="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
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
                        className="h-4 w-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground"
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

function RequiredConnectionsSection({
  requirements,
  missing,
  integrationsLoading,
  isBusy,
  onConnect,
}: {
  requirements: PluginRequirement[]
  missing: PluginRequirement[]
  integrationsLoading: boolean
  isBusy: boolean
  onConnect: (requirement: PluginRequirement) => void
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-base font-semibold text-foreground">
          Required connections
        </h2>
        {missing.length === 0 ? (
          <p className="text-sm leading-5 text-muted-foreground">
            All required connections are connected.
          </p>
        ) : null}
      </div>
      {missing.length > 0 ? (
        <div className="border-warning/40 bg-warning/10 flex gap-3 rounded-xl border p-4">
          <div className="bg-warning/15 text-warning flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
            <Icon icon="lucide:triangle-alert" className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <h3 className="text-sm font-medium text-foreground">
              Required connections missing
            </h3>
            <p className="mt-1 text-sm leading-5 text-muted-foreground">
              Add the required connections before adding this plugin.
            </p>
          </div>
        </div>
      ) : null}
      <div className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border bg-card">
        {requirements.map((requirement, index) => {
          const provider = requirement.provider ?? ""
          const isMissing = isRequirementMissing(requirement, missing)
          const canConnect =
            isMissing &&
            (isDatabaseRequirement(requirement) ||
              isIntegrationRequirement(requirement))
          const waitingForIntegrations =
            integrationsLoading && isIntegrationRequirement(requirement)

          return (
            <div
              key={provider || index}
              className="flex items-center justify-between gap-3 px-3 py-2.5"
            >
              <div className="flex min-w-0 items-center gap-3">
                <RequirementLogo requirement={requirement} />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-foreground">
                    {provider ? providerLabel(provider) : "Connection"}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {connectionKindLabel(requirement)}
                  </p>
                </div>
              </div>
              {isMissing ? (
                <Button
                  size="sm"
                  variant="primary"
                  className="shrink-0 rounded-full"
                  isDisabled={!canConnect || isBusy || waitingForIntegrations}
                  onPress={() => onConnect(requirement)}
                >
                  {isBusy ? <Spinner color="current" size="sm" /> : null}
                  Connect
                </Button>
              ) : (
                <span
                  aria-label="Connected"
                  className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-success text-success-foreground"
                >
                  <Icon icon="lucide:check" className="h-3.5 w-3.5" />
                </span>
              )}
            </div>
          )
        })}
      </div>
    </section>
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
      <div className="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
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
              <p className="text-sm leading-5 text-muted-foreground">
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
          <div className="bg-default h-12 w-12 animate-pulse rounded-xl" />
          <div className="flex flex-col gap-3">
            <div className="bg-default h-5 w-36 animate-pulse rounded" />
            <div className="bg-default h-4 w-80 max-w-full animate-pulse rounded" />
          </div>
        </div>
        <div className="bg-default h-8 w-16 animate-pulse rounded-full" />
      </header>
      <div className="bg-default h-40 animate-pulse rounded-xl" />
      <div className="bg-default h-56 animate-pulse rounded-xl" />
    </div>
  )
}
