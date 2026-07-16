"use client"

import { useMemo, useState } from "react"
import { Input, Modal, Skeleton, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  type AvailableIntegration,
  integrationNeedsForm,
} from "./integration-auth"
import { IntegrationCredentialsForm } from "./integration-credentials-form"
import {
  type ConnectOptions,
  useConnectIntegration,
} from "./use-connect-integration"

type Connection = components["schemas"]["connectionResponse"]

const EMPTY_CONNECTIONS: Connection[] = []
const EMPTY_INTEGRATIONS: AvailableIntegration[] = []

const CONNECTION_DESCRIPTIONS: Record<string, string> = {
  slack: "Work with channels, messages, and your team in Slack.",
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

export default function ConnectionsPage() {
  const [query, setQuery] = useState("")
  const [formIntegration, setFormIntegration] =
    useState<AvailableIntegration | null>(null)
  const integrationsQuery = $api.useQuery("get", "/v1/integrations/available")
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const integrations = integrationsQuery.data?.data ?? EMPTY_INTEGRATIONS
  const connections = connectionsQuery.data?.data ?? EMPTY_CONNECTIONS
  const filteredIntegrations = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    if (!normalizedQuery) return integrations

    return integrations.filter((integration) => {
      const provider = integration.provider ?? ""
      const description = CONNECTION_DESCRIPTIONS[provider] ?? ""
      return `${integration.display_name ?? ""} ${provider} ${description}`
        .toLowerCase()
        .includes(normalizedQuery)
    })
  }, [integrations, query])
  const { connectIntegration, connectingId, isConnecting } =
    useConnectIntegration()

  function connect(
    integration: AvailableIntegration,
    options?: ConnectOptions
  ) {
    if (!integration.id) return

    connectIntegration(integration.id, {
      ...options,
      onSuccess: () => {
        setFormIntegration(null)
        toast.success(
          `${integration.display_name ?? "Connection"} is ready to use`
        )
        void connectionsQuery.refetch()
      },
    })
  }

  function requestConnect(integration: AvailableIntegration) {
    if (integrationNeedsForm(integration)) {
      setFormIntegration(integration)
      return
    }

    connect(integration)
  }

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Connections</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Connect the tools your teams and agents use.
        </p>
      </div>

      <div className="relative min-w-0 flex-1">
        <AppIcon
          icon="search"
          className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2"
        />
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search connections"
          aria-label="Search connections"
          className="bg-card h-10 w-full rounded-md pl-9"
        />
      </div>

      <ConnectedConnectionsSection
        connections={connections}
        isLoading={connectionsQuery.isLoading}
      />

      {integrationsQuery.isLoading ? (
        <CatalogSkeleton />
      ) : integrationsQuery.isError ? (
        <ErrorState />
      ) : filteredIntegrations.length === 0 ? (
        <EmptyState query={query} hasIntegrations={integrations.length > 0} />
      ) : (
        <section className="flex flex-col gap-3">
          <h2 className="text-sm font-medium text-foreground">
            Available connections
          </h2>
          <div className="bg-card flex flex-col">
            {filteredIntegrations.map((integration, index) => (
              <ConnectionRow
                key={integration.id ?? integration.provider ?? index}
                integration={integration}
                isConnecting={connectingId === integration.id}
                isDisabled={isConnecting}
                onConnect={() => requestConnect(integration)}
              />
            ))}
          </div>
        </section>
      )}

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

function ConnectedConnectionsSection({
  connections,
  isLoading,
}: {
  connections: Connection[]
  isLoading: boolean
}) {
  if (isLoading) {
    return (
      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium text-foreground">Connected</h2>
        <div className="flex flex-wrap items-center gap-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-8 w-8" />
          ))}
        </div>
      </section>
    )
  }

  if (connections.length === 0) return null

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-medium text-foreground">Connected</h2>
      <div className="flex flex-wrap items-center gap-2">
        {connections.map((connection, index) => (
          <div
            key={
              connection.id ?? `${connection.provider ?? "connection"}-${index}`
            }
            className="bg-card flex h-8 w-8 items-center justify-center rounded-lg transition-colors hover:bg-muted/40"
            title={
              connection.name ?? connection.display_name ?? connection.provider
            }
          >
            <IntegrationLogo provider={connection.provider ?? ""} size={24} />
          </div>
        ))}
      </div>
    </section>
  )
}

function ConnectionRow({
  integration,
  isConnecting,
  isDisabled,
  onConnect,
}: {
  integration: AvailableIntegration
  isConnecting: boolean
  isDisabled: boolean
  onConnect: () => void
}) {
  const provider = integration.provider ?? ""

  return (
    <button
      type="button"
      className="group -mx-3 block py-1.5 text-left disabled:cursor-wait disabled:opacity-60"
      disabled={isDisabled}
      onClick={onConnect}
    >
      <div className="rounded-xl px-3 py-1.5 transition-colors group-hover:bg-default group-focus-visible:bg-default">
        <div className="flex items-center gap-3">
          <IntegrationLogo provider={provider} size={36} />
          <div className="min-w-0 flex-1">
            <h3 className="truncate text-sm font-medium text-foreground">
              {integration.display_name || provider}
            </h3>
            <p className="text-muted-foreground truncate text-sm">
              {CONNECTION_DESCRIPTIONS[provider] ??
                "Connect this tool to use it with Hivy."}
            </p>
          </div>
          {isConnecting ? (
            <Spinner color="current" size="sm" />
          ) : (
            <AppIcon
              icon="chevron-right"
              className="text-muted-foreground h-4 w-4 shrink-0 transition-colors group-hover:text-foreground"
            />
          )}
        </div>
      </div>
    </button>
  )
}

function CatalogSkeleton() {
  return (
    <section className="flex flex-col gap-3">
      <Skeleton className="h-4 w-36 rounded" />
      <div className="bg-card flex flex-col gap-3">
        {Array.from({ length: 5 }).map((_, row) => (
          <div key={row} className="flex items-center gap-3 py-1.5">
            <Skeleton className="h-9 w-9" />
            <div className="min-w-0 flex-1">
              <Skeleton className="h-4 w-40 rounded" />
              <Skeleton className="mt-2 h-4 w-full max-w-sm rounded" />
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

function ErrorState() {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon
        icon="triangle-alert"
        className="text-muted-foreground h-7 w-7"
      />
      <p className="mt-3 text-sm font-medium text-foreground">
        Could not load connections
      </p>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">
        Refresh the page to try again.
      </p>
    </div>
  )
}

function EmptyState({
  query,
  hasIntegrations,
}: {
  query: string
  hasIntegrations: boolean
}) {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon icon="plug" className="text-muted-foreground h-7 w-7" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query && hasIntegrations
          ? "No matching connections"
          : "No connections available"}
      </p>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">
        {query && hasIntegrations
          ? "Try a different search."
          : "Configure an integration in Nango to add it to the catalog."}
      </p>
    </div>
  )
}
