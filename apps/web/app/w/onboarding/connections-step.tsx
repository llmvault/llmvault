"use client"

import posthog from "posthog-js"
import { useMemo, useState } from "react"
import { Button, Input, Modal, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  type AvailableIntegration,
  integrationNeedsForm,
} from "@/app/w/(chat)/connections/integration-auth"
import { IntegrationCredentialsForm } from "@/app/w/(chat)/connections/integration-credentials-form"
import {
  type ConnectOptions,
  useConnectIntegration,
} from "@/app/w/(chat)/connections/use-connect-integration"

type Connection = components["schemas"]["connectionResponse"]

const CONNECTION_DESCRIPTIONS: Record<string, string> = {
  slack: "Work with Slack channels, messages, and your team in Slack.",
  "github-app": "Read repositories, issues, and pull requests.",
  "github-app-code-reviews":
    "Review pull requests and respond to code changes.",
  notion: "Use pages and databases as company context.",
  linear: "Create, update, and track product work.",
  railway: "Inspect and manage Railway projects and deployments.",
  vercel: "Work with projects, deployments, and domains.",
  apify: "Run actors and use web data in your workflows.",
  bugsink: "Investigate errors and application issues.",
  glitchtip: "Inspect errors, events, and performance data.",
}

export function ConnectionsStep({
  onContinue,
  advancing,
  showContinue = true,
}: {
  onContinue?: () => void
  advancing?: boolean
  showContinue?: boolean
}) {
  const integrationsQuery = $api.useQuery("get", "/v1/integrations/available")
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const [search, setSearch] = useState("")
  const integrations = useMemo(
    () => integrationsQuery.data?.data ?? [],
    [integrationsQuery.data?.data]
  )
  const connections = useMemo(
    () => connectionsQuery.data?.data ?? [],
    [connectionsQuery.data?.data]
  )
  const connectedProviders = useMemo(
    () =>
      new Set(
        connections.flatMap((connection: Connection) =>
          connection.provider ? [connection.provider] : []
        )
      ),
    [connections]
  )
  const filteredIntegrations = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return integrations
    return integrations.filter((integration) =>
      `${integration.display_name ?? ""} ${integration.provider ?? ""}`
        .toLowerCase()
        .includes(query)
    )
  }, [integrations, search])
  const { connectIntegration, connectingId, isConnecting } =
    useConnectIntegration()
  const [formIntegration, setFormIntegration] =
    useState<AvailableIntegration | null>(null)

  function connect(
    integration: AvailableIntegration,
    options?: ConnectOptions
  ) {
    if (!integration.id) return
    connectIntegration(integration.id, {
      ...options,
      onSuccess: () => {
        posthog.capture("onboarding_connection_created", {
          provider: integration.provider,
          integration_name: integration.display_name,
        })
        setFormIntegration(null)
        toast.success(
          `${integration.display_name ?? "Connection"} is ready to use`
        )
        void connectionsQuery.refetch()
      },
    })
  }

  function requestConnect(integration: AvailableIntegration) {
    if (integrationNeedsForm(integration)) setFormIntegration(integration)
    else connect(integration)
  }

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-1 flex-col">
      <div className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
            Connections
          </h1>
          <p className="mt-4 max-w-2xl text-base leading-7 text-muted">
            Give your agents restricted access to the tools your team uses.
          </p>
        </div>
        {connections.length > 0 ? (
          <div className="flex shrink-0 items-center gap-2 text-sm font-medium text-foreground">
            <span className="flex h-6 w-6 items-center justify-center rounded-full bg-success/15 text-success">
              <AppIcon icon="check" className="h-3.5 w-3.5" />
            </span>
            {connections.length} connected
          </div>
        ) : null}
      </div>

      <div className="mt-8 flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border bg-surface">
        <div className="border-b border-border p-3">
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search connections"
            aria-label="Search connections"
            className="w-full"
          />
        </div>

        <div className="min-h-64 flex-1 overflow-y-auto">
          {integrationsQuery.isLoading ? (
            <ConnectionSkeletons />
          ) : integrationsQuery.isError ? (
            <ConnectionState
              icon="circle-alert"
              title="Connections could not be loaded"
              description="Check your network and try again. You can also finish setup without connecting a tool."
              action={
                <Button
                  size="sm"
                  variant="secondary"
                  onPress={() => integrationsQuery.refetch()}
                >
                  Try again
                </Button>
              }
            />
          ) : integrations.length === 0 ? (
            <ConnectionState
              icon="plug"
              title="No connections are ready yet"
              description="Configure an integration in Nango, then restart Hivy to refresh the connection catalog. You can continue and add tools later."
            />
          ) : filteredIntegrations.length === 0 ? (
            <ConnectionState
              icon="search"
              title="No matching connections"
              description={`Nothing matches “${search.trim()}”. Try another name.`}
            />
          ) : (
            filteredIntegrations.map((integration, index) => {
              const provider = integration.provider ?? ""
              const connected = connectedProviders.has(provider)
              const connecting = connectingId === integration.id
              return (
                <div
                  key={integration.id ?? provider}
                  className={`grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-3 px-4 py-4 transition-colors hover:bg-default/60 sm:flex ${
                    index ? "border-t border-border" : ""
                  }`}
                >
                  <IntegrationLogo
                    provider={provider}
                    size={40}
                    className="shrink-0 rounded-xl"
                  />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold">
                      {integration.display_name || provider}
                    </p>
                    <p className="mt-1 line-clamp-1 text-xs text-muted">
                      {connected
                        ? "Connected and ready for your team"
                        : (CONNECTION_DESCRIPTIONS[provider] ??
                          "Connect this tool to use it with Hivy.")}
                    </p>
                  </div>
                  <Button
                    className="col-start-2 justify-self-start sm:col-auto sm:justify-self-auto"
                    size="sm"
                    variant={connected ? "tertiary" : "secondary"}
                    isDisabled={isConnecting}
                    onPress={() => requestConnect(integration)}
                  >
                    {connecting ? <Spinner color="current" size="sm" /> : null}
                    {connected ? "Add another" : "Connect"}
                  </Button>
                </div>
              )
            })
          )}
        </div>
      </div>

      {showContinue ? (
        <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          {connections.length === 0 ? (
            <p className="text-xs text-muted">
              Add at least one connection to continue.
            </p>
          ) : null}
          <Button
            className="w-full sm:ml-auto sm:w-auto"
            variant="primary"
            onPress={onContinue ?? (() => undefined)}
            isDisabled={connections.length === 0 || advancing || isConnecting}
          >
            {advancing ? <Spinner color="current" size="sm" /> : null}
            Continue
            <AppIcon icon="arrow-right" className="h-4 w-4" />
          </Button>
        </div>
      ) : null}

      <Modal
        isOpen={formIntegration !== null}
        onOpenChange={(open) => !open && setFormIntegration(null)}
      >
        <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
          <Modal.Container placement="center" className="p-4">
            <Modal.Dialog className="w-full max-w-sm bg-background p-0 shadow-xl outline-none">
              {formIntegration ? (
                <IntegrationCredentialsForm
                  integration={formIntegration}
                  isSubmitting={isConnecting}
                  onBack={() => setFormIntegration(null)}
                  onSubmit={(options) => connect(formIntegration, options)}
                />
              ) : null}
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </div>
  )
}

function ConnectionSkeletons() {
  return (
    <div aria-label="Loading connections">
      {[0, 1, 2, 3].map((item) => (
        <div
          key={item}
          className="grid animate-pulse grid-cols-[2.5rem_minmax(0,1fr)] items-center gap-x-3 gap-y-3 border-b border-border px-4 py-4 last:border-b-0 sm:flex"
        >
          <div className="h-10 w-10 rounded-xl bg-default" />
          <div className="min-w-0 flex-1">
            <div className="h-3 w-28 rounded-full bg-default" />
            <div className="mt-2 h-2.5 w-64 max-w-full rounded-full bg-default" />
          </div>
          <div className="col-start-2 h-8 w-20 rounded-lg bg-default sm:col-auto" />
        </div>
      ))}
    </div>
  )
}

function ConnectionState({
  icon,
  title,
  description,
  action,
}: {
  icon: string
  title: string
  description: string
  action?: React.ReactNode
}) {
  return (
    <div className="flex min-h-64 flex-col items-center justify-center px-6 py-10 text-center">
      <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-default text-muted">
        <AppIcon icon={icon} className="h-5 w-5" />
      </div>
      <p className="mt-4 text-sm font-semibold">{title}</p>
      <p className="mt-1.5 max-w-sm text-sm leading-6 text-muted">
        {description}
      </p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  )
}
