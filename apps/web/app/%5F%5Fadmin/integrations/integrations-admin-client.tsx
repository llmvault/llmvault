"use client"

import { useEffect, useMemo, useState, type FormEvent } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  CheckmarkCircle02Icon,
  Plug01Icon,
  RefreshIcon,
  Search01Icon,
  Settings05Icon,
} from "@hugeicons/core-free-icons"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"
import { IntegrationLogo } from "@/components/integration-logo"
import { FullPageLoader } from "@/components/full-page-loader"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

type AdminIntegrationDefinition = components["schemas"]["AdminDefinition"]
type Credentials = components["schemas"]["Credentials"]

const EMPTY_DEFINITIONS: AdminIntegrationDefinition[] = []

export function AdminIntegrationsClient() {
  return (
    <AuthProvider>
      <AdminIntegrationsContent />
    </AuthProvider>
  )
}

function AdminIntegrationsContent() {
  const queryClient = useQueryClient()
  const { isLoading: authLoading, isPlatformAdmin } = useAuth()
  const [adminSecretDraft, setAdminSecretDraft] = useState("")
  const [adminSecret, setAdminSecret] = useState("")
  const [search, setSearch] = useState("")
  const [selected, setSelected] = useState<AdminIntegrationDefinition | null>(
    null
  )

  const adminSecretHeader = adminSecret.trim()
  const hasAdminSecret = adminSecretHeader.length > 0
  const adminRequest = useMemo(
    () => ({
      params: {
        header: {
          "X-Hivy-Admin-Secret": adminSecretHeader,
        },
      },
    }),
    [adminSecretHeader]
  )

  const definitionsQuery = $api.useQuery(
    "get",
    "/v1/admin/integrations",
    adminRequest,
    {
      enabled: !authLoading && isPlatformAdmin && hasAdminSecret,
      retry: false,
    }
  )

  useEffect(() => {
    if (definitionsQuery.error) {
      toast.error(
        extractErrorMessage(
          definitionsQuery.error,
          "Failed to load integrations"
        )
      )
    }
  }, [definitionsQuery.error])

  const definitions = definitionsQuery.data ?? EMPTY_DEFINITIONS
  const filteredDefinitions = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return definitions
    return definitions.filter((definition) => {
      return [
        definition.display_name,
        definition.provider,
        definition.unique_key,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(query)
    })
  }, [definitions, search])

  if (authLoading) return <FullPageLoader />

  if (!isPlatformAdmin) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background p-6">
        <div className="flex max-w-sm items-center gap-3 rounded-md border border-border bg-card p-4 text-sm text-muted-foreground">
          <HugeiconsIcon icon={Alert02Icon} size={20} strokeWidth={1.8} />
          <span>Admin access required.</span>
        </div>
      </main>
    )
  }

  function handleSecretSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setAdminSecret(adminSecretDraft.trim())
  }

  return (
    <main className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border/70 px-6 py-4">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-card">
              <HugeiconsIcon
                icon={Settings05Icon}
                size={19}
                strokeWidth={1.8}
              />
            </div>
            <div className="min-w-0">
              <p className="text-xs font-medium tracking-[0.14em] text-muted-foreground uppercase">
                Admin
              </p>
              <h1 className="truncate text-lg font-semibold">Integrations</h1>
            </div>
          </div>
          {hasAdminSecret ? (
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => void definitionsQuery.refetch()}
              >
                <HugeiconsIcon icon={RefreshIcon} size={16} strokeWidth={1.8} />
                Refresh
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setAdminSecret("")
                  setAdminSecretDraft("")
                  setSelected(null)
                  queryClient.removeQueries({
                    queryKey: ["get", "/v1/admin/integrations"],
                  })
                }}
              >
                Change secret
              </Button>
            </div>
          ) : null}
        </div>
      </header>

      {!hasAdminSecret ? (
        <section className="mx-auto flex max-w-6xl px-6 py-6">
          <form
            onSubmit={handleSecretSubmit}
            className="grid w-full max-w-sm gap-3 rounded-md border border-border bg-card p-4"
          >
            <Label htmlFor="admin-secret">Admin secret</Label>
            <Input
              id="admin-secret"
              type="password"
              autoComplete="off"
              value={adminSecretDraft}
              onChange={(event) => setAdminSecretDraft(event.target.value)}
            />
            <Button
              type="submit"
              disabled={adminSecretDraft.trim().length === 0}
            >
              Continue
            </Button>
          </form>
        </section>
      ) : (
        <section className="mx-auto flex max-w-6xl flex-col gap-5 px-6 py-6">
          <div className="relative max-w-md">
            <HugeiconsIcon
              icon={Search01Icon}
              size={17}
              strokeWidth={1.8}
              className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search integrations"
              className="pl-9"
            />
          </div>

          <div className="grid gap-3">
            {definitionsQuery.isLoading ? (
              Array.from({ length: 6 }).map((_, index) => (
                <Skeleton key={index} className="h-20 rounded-md" />
              ))
            ) : filteredDefinitions.length > 0 ? (
              filteredDefinitions.map((definition, index) => (
                <IntegrationRow
                  key={
                    definition.id ??
                    definition.unique_key ??
                    definition.provider ??
                    definition.display_name ??
                    index
                  }
                  definition={definition}
                  onSelect={() => setSelected(definition)}
                />
              ))
            ) : (
              <div className="rounded-md border border-border bg-card p-8 text-center text-sm text-muted-foreground">
                No integrations found.
              </div>
            )}
          </div>
        </section>
      )}

      <IntegrationDialog
        key={selected?.id ?? "empty"}
        adminSecret={adminSecretHeader}
        definition={selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null)
        }}
        onSaved={() => {
          setSelected(null)
          void queryClient.invalidateQueries({
            queryKey: ["get", "/v1/admin/integrations"],
          })
        }}
      />
    </main>
  )
}

function IntegrationRow({
  definition,
  onSelect,
}: {
  definition: AdminIntegrationDefinition
  onSelect: () => void
}) {
  const displayName = definition.display_name ?? "Untitled integration"
  const provider = definition.provider ?? ""
  const uniqueKey = definition.unique_key ?? "unknown"
  const authMode = definition.auth_mode || "No auth"

  return (
    <button
      type="button"
      onClick={onSelect}
      className="group flex min-h-20 items-center justify-between gap-4 rounded-md border border-border bg-card px-4 py-3 text-left transition-colors hover:border-foreground/25 hover:bg-accent/40"
    >
      <div className="flex min-w-0 items-center gap-3">
        <IntegrationLogo
          provider={provider}
          className="h-10 w-10 shrink-0 rounded-md border border-border bg-background p-2"
        />
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <p className="truncate text-sm font-medium">{displayName}</p>
            {definition.existing ? (
              <Badge variant="secondary" className="shrink-0">
                Created
              </Badge>
            ) : (
              <Badge variant="outline" className="shrink-0">
                Not created
              </Badge>
            )}
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {uniqueKey} · {authMode}
          </p>
        </div>
      </div>
      <div className="hidden items-center gap-2 sm:flex">
        {definition.supports_rag_source ? (
          <Badge variant="outline">RAG</Badge>
        ) : null}
        <Button
          size="sm"
          variant={definition.existing ? "outline" : "default"}
          tabIndex={-1}
        >
          <HugeiconsIcon icon={Plug01Icon} size={15} strokeWidth={1.8} />
          {definition.existing ? "Update" : "Create"}
        </Button>
      </div>
    </button>
  )
}

function IntegrationDialog({
  adminSecret,
  definition,
  onOpenChange,
  onSaved,
}: {
  adminSecret: string
  definition: AdminIntegrationDefinition | null
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}) {
  const [values, setValues] = useState<Record<string, string>>({})
  const upsertIntegration = $api.useMutation(
    "put",
    "/v1/admin/integrations/{id}"
  )

  if (!definition) return null

  const definitionID = definition.id ?? ""
  const displayName = definition.display_name ?? "Untitled integration"
  const provider = definition.provider ?? ""
  const fields = definition.credential_fields ?? []
  const hasFields = fields.length > 0

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!definitionID) {
      toast.error("Integration definition is missing an ID")
      return
    }

    try {
      const credentials = hasFields ? (values as Credentials) : undefined
      const data = await upsertIntegration.mutateAsync({
        params: {
          path: { id: definitionID },
          header: { "X-Hivy-Admin-Secret": adminSecret },
        },
        body: { credentials },
      })
      toast.success(
        data.state === "created"
          ? "Integration created"
          : data.state === "updated"
            ? "Integration updated"
            : "Integration synced"
      )
      onSaved()
    } catch (error) {
      toast.error(extractErrorMessage(error, "Failed to sync integration"))
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-3">
            <IntegrationLogo
              provider={provider}
              className="h-9 w-9 rounded-md border border-border bg-background p-2"
            />
            <span className="min-w-0 truncate">{displayName}</span>
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="grid gap-2 rounded-md border border-border bg-muted/30 p-3 text-sm">
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground">Provider key</span>
              <span className="truncate font-medium">
                {definition.unique_key ?? "unknown"}
              </span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground">Auth mode</span>
              <span className="truncate font-medium">
                {definition.auth_mode || "None"}
              </span>
            </div>
            {definition.existing ? (
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Connections</span>
                <span className="truncate font-medium">
                  {definition.existing.active_connections ?? 0}
                </span>
              </div>
            ) : null}
          </div>

          {(definition.fixed_credentials ?? []).length > 0 ? (
            <div className="grid gap-2">
              {definition.fixed_credentials?.map((credential) => (
                <div
                  key={`${credential.name ?? ""}-${credential.value ?? ""}`}
                  className="flex items-start justify-between gap-3 rounded-md bg-muted/35 px-3 py-2 text-sm"
                >
                  <span className="shrink-0 text-muted-foreground">
                    {credential.label ?? credential.name}
                  </span>
                  <span className="min-w-0 text-right font-medium break-words">
                    {credential.value}
                  </span>
                </div>
              ))}
            </div>
          ) : null}

          {hasFields ? (
            <div className="grid gap-4">
              {fields.map((field) => {
                const fieldName = field.name ?? ""
                return (
                  <div key={fieldName} className="grid gap-2">
                    <Label htmlFor={`admin-integration-${fieldName}`}>
                      {field.label ?? field.name}
                    </Label>
                    {field.multiline ? (
                      <Textarea
                        id={`admin-integration-${fieldName}`}
                        required={field.required}
                        value={values[fieldName] ?? ""}
                        onChange={(event) =>
                          setValues((current) => ({
                            ...current,
                            [fieldName]: event.target.value,
                          }))
                        }
                        placeholder={field.placeholder}
                        className="min-h-32 font-mono text-xs"
                      />
                    ) : (
                      <Input
                        id={`admin-integration-${fieldName}`}
                        required={field.required}
                        type={field.secret ? "password" : "text"}
                        value={values[fieldName] ?? ""}
                        onChange={(event) =>
                          setValues((current) => ({
                            ...current,
                            [fieldName]: event.target.value,
                          }))
                        }
                        placeholder={field.placeholder}
                      />
                    )}
                  </div>
                )
              })}
            </div>
          ) : (
            <div className="flex items-center gap-2 rounded-md border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
              <HugeiconsIcon
                icon={CheckmarkCircle02Icon}
                size={18}
                strokeWidth={1.8}
              />
              <span>No platform credentials required.</span>
            </div>
          )}

          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={upsertIntegration.isPending}
              className="min-w-24"
            >
              {upsertIntegration.isPending
                ? "Saving"
                : definition.existing
                  ? "Update"
                  : "Create"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
