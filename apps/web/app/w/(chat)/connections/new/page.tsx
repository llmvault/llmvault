"use client"

import { useMemo, useState } from "react"
import NextLink from "next/link"
import { Input, Modal, Skeleton, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { useIsAdmin } from "@/lib/auth/use-role"
import {
  ConnectionNameModal,
  type ConnectionNameTarget,
} from "../connection-name-modal"
import {
  DatabaseConnectionModalContent,
  type DatabaseProvider,
} from "../database-connection-modal-content"
import {
  type AvailableIntegration,
  integrationNeedsForm,
} from "../integration-auth"
import { IntegrationCredentialsForm } from "../integration-credentials-form"
import {
  type ConnectOptions,
  useConnectIntegration,
} from "../use-connect-integration"

interface DatabaseCatalogEntry {
  provider: DatabaseProvider
  name: string
  description: string
}

const EMPTY_INTEGRATIONS: AvailableIntegration[] = []

const DATABASES: DatabaseCatalogEntry[] = [
  {
    provider: "postgres",
    name: "PostgreSQL",
    description: "Connect a PostgreSQL database with controlled access.",
  },
  {
    provider: "mysql",
    name: "MySQL",
    description: "Connect a MySQL database with controlled access.",
  },
  {
    provider: "mongodb",
    name: "MongoDB",
    description: "Connect MongoDB collections with controlled access.",
  },
  {
    provider: "redis",
    name: "Redis",
    description: "Connect Redis keys with controlled access.",
  },
]

const DESCRIPTIONS: Record<string, string> = {
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

export default function AddConnectionPage() {
  const isAdmin = useIsAdmin()
  const [query, setQuery] = useState("")
  const [integration, setIntegration] = useState<AvailableIntegration | null>(
    null
  )
  const [database, setDatabase] = useState<DatabaseCatalogEntry | null>(null)
  const [nameTarget, setNameTarget] = useState<ConnectionNameTarget | null>(
    null
  )
  const integrationsQuery = $api.useQuery("get", "/v1/integrations/available")
  const integrations = integrationsQuery.data?.data ?? EMPTY_INTEGRATIONS
  const filteredIntegrations = useMemo(
    () => filterCatalog(integrations, query),
    [integrations, query]
  )
  const filteredDatabases = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    if (!normalized) return DATABASES
    return DATABASES.filter((entry) =>
      `${entry.name} ${entry.provider} ${entry.description}`
        .toLowerCase()
        .includes(normalized)
    )
  }, [query])
  const { connectIntegration, connectingId, isConnecting } =
    useConnectIntegration()

  function connected(
    connection: {
      id?: string
      name?: string
      slug?: string
      needs_name?: boolean
    },
    kind: "integration" | "database"
  ) {
    if (connection.needs_name && connection.id) {
      setNameTarget({
        id: connection.id,
        kind,
        name: connection.name ?? connection.slug ?? "",
        needsName: true,
      })
    }
  }

  function connect(selected: AvailableIntegration, options?: ConnectOptions) {
    if (!selected.id) return
    connectIntegration(selected.id, {
      ...options,
      onSuccess: (connection) => {
        setIntegration(null)
        toast.success(`${selected.display_name ?? "Connection"} connected`)
        connected(connection, "integration")
      },
    })
  }

  function selectIntegration(selected: AvailableIntegration) {
    if (!isAdmin || !selected.id) return
    if (integrationNeedsForm(selected)) {
      setIntegration(selected)
      return
    }
    connect(selected)
  }

  return (
    <div className="flex flex-col gap-8">
      <NextLink
        href="/w/connections"
        className="text-muted-foreground flex w-fit items-center gap-2 text-sm transition-colors hover:text-foreground"
      >
        <AppIcon icon="arrow-left" className="h-4 w-4" />
        Connections
      </NextLink>

      <div>
        <h1 className="text-lg font-semibold text-foreground">
          Add connection
        </h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Choose an integration to connect to your workspace.
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
          placeholder="Search integrations"
          aria-label="Search integrations"
          className="bg-card h-10 w-full rounded-md pl-9"
        />
      </div>

      {filteredDatabases.length > 0 ? (
        <CatalogSection title="Databases">
          {filteredDatabases.map((entry) => (
            <CatalogRow
              key={entry.provider}
              provider={entry.provider}
              name={entry.name}
              description={entry.description}
              disabled={!isAdmin}
              onSelect={() => setDatabase(entry)}
            />
          ))}
        </CatalogSection>
      ) : null}

      {integrationsQuery.isLoading ? (
        <CatalogSkeleton />
      ) : integrationsQuery.isError ? (
        <ErrorState />
      ) : filteredIntegrations.length > 0 ? (
        <CatalogSection title="Integrations">
          {filteredIntegrations.map((entry, index) => (
            <CatalogRow
              key={entry.id ?? entry.provider ?? index}
              provider={entry.provider ?? ""}
              name={entry.display_name || entry.provider || "Integration"}
              description={
                DESCRIPTIONS[entry.provider ?? ""] ??
                "Connect this integration to use it with Hivy."
              }
              disabled={!isAdmin || isConnecting}
              loading={connectingId === entry.id}
              onSelect={() => selectIntegration(entry)}
            />
          ))}
        </CatalogSection>
      ) : (
        <EmptyState query={query} hasIntegrations={integrations.length > 0} />
      )}

      {!isAdmin ? (
        <p className="text-muted-foreground text-sm">
          Only workspace admins can add connections.
        </p>
      ) : null}

      <Modal
        isOpen={integration !== null}
        onOpenChange={(open) => !open && setIntegration(null)}
      >
        <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
          <Modal.Container placement="center" className="p-4">
            <Modal.Dialog className="w-full max-w-sm bg-background p-0 shadow-xl outline-none">
              {integration ? (
                <IntegrationCredentialsForm
                  integration={integration}
                  isSubmitting={isConnecting}
                  onBack={() => setIntegration(null)}
                  onSubmit={(options) => connect(integration, options)}
                />
              ) : null}
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      <Modal
        isOpen={database !== null}
        onOpenChange={(open) => !open && setDatabase(null)}
      >
        <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
          <Modal.Container placement="center" className="p-4">
            <Modal.Dialog className="w-full max-w-3xl bg-background p-0 shadow-xl outline-none">
              {database ? (
                <DatabaseConnectionModalContent
                  provider={database.provider}
                  canManage={isAdmin}
                  onConnected={(connection) => {
                    setDatabase(null)
                    connected(connection, "database")
                  }}
                />
              ) : null}
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      {nameTarget ? (
        <ConnectionNameModal
          key={`${nameTarget.kind}:${nameTarget.id}:${nameTarget.name}`}
          target={nameTarget}
          onClose={() => setNameTarget(null)}
          onSaved={() => setNameTarget(null)}
        />
      ) : null}
    </div>
  )
}

function CatalogSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-medium text-foreground">{title}</h2>
      <div className="bg-card flex flex-col">{children}</div>
    </section>
  )
}

function CatalogRow({
  provider,
  name,
  description,
  disabled,
  loading = false,
  onSelect,
}: {
  provider: string
  name: string
  description: string
  disabled: boolean
  loading?: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onSelect}
      className="group -mx-3 block py-1.5 text-left disabled:cursor-not-allowed disabled:opacity-60"
    >
      <div className="rounded-xl px-3 py-1.5 transition-colors group-hover:bg-default group-focus-visible:bg-default">
        <div className="flex items-center gap-3">
          <IntegrationLogo provider={provider} size={36} />
          <div className="min-w-0 flex-1">
            <h3 className="truncate text-sm font-medium text-foreground">
              {name}
            </h3>
            <p className="text-muted-foreground truncate text-sm">
              {description}
            </p>
          </div>
          {loading ? (
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

function filterCatalog(
  values: AvailableIntegration[],
  query: string
): AvailableIntegration[] {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return values
  return values.filter((entry) => {
    const provider = entry.provider ?? ""
    return `${entry.display_name ?? ""} ${provider} ${DESCRIPTIONS[provider] ?? ""}`
      .toLowerCase()
      .includes(normalized)
  })
}

function CatalogSkeleton() {
  return (
    <section className="flex flex-col gap-3">
      <Skeleton className="h-4 w-24 rounded" />
      <div className="bg-card flex flex-col gap-3">
        {Array.from({ length: 8 }).map((_, index) => (
          <div key={index} className="flex items-center gap-3 py-1.5">
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
        Could not load integrations
      </p>
      <p className="text-muted-foreground mt-1 text-sm">
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
          ? "No matching integrations"
          : "No integrations available"}
      </p>
      <p className="text-muted-foreground mt-1 text-sm">
        {query && hasIntegrations
          ? "Try a different search."
          : "Configure an integration in Nango to add it to the catalog."}
      </p>
    </div>
  )
}
