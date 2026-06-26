"use client"

import { useEffect, useMemo, useState, type FormEvent } from "react"
import type { QueryClient } from "@tanstack/react-query"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { ADMIN_QUERY_KEYS, adminSecretHeader, errorMessage } from "./admin-api"
import { AdminContentHeader } from "./admin-content-header"
import { SecretGate } from "./admin-secret-gate"
import { AdminSidebar } from "./admin-sidebar"
import { CredentialPanel } from "./credentials-admin"
import { IntegrationDialog } from "./integration-dialog"
import { IntegrationList } from "./integrations-admin"
import {
  emptyCredentialForm,
  type AdminIntegrationDefinition,
  type AdminTab,
  type CredentialForm,
  type LLMProvider,
  type SystemCredential,
} from "./types"

const EMPTY_INTEGRATIONS: AdminIntegrationDefinition[] = []
const EMPTY_CREDENTIALS: SystemCredential[] = []
const EMPTY_PROVIDERS: LLMProvider[] = []

export function AdminClient() {
  const queryClient = useQueryClient()
  const [secretDraft, setSecretDraft] = useState("")
  const [adminSecret, setAdminSecret] = useState("")
  const [activeTab, setActiveTab] = useState<AdminTab>("integrations")
  const [search, setSearch] = useState("")
  const [selectedIntegration, setSelectedIntegration] =
    useState<AdminIntegrationDefinition | null>(null)
  const [credentialForm, setCredentialForm] =
    useState<CredentialForm>(emptyCredentialForm)

  const hasSecret = adminSecret.trim().length > 0
  const adminHeaders = useMemo(
    () => adminSecretHeader(adminSecret),
    [adminSecret]
  )
  const integrationsQuery = $api.useQuery(
    "get",
    "/v1/admin/integrations",
    { params: { header: adminHeaders } },
    { enabled: hasSecret, retry: false }
  )
  const credentialsQuery = $api.useQuery(
    "get",
    "/v1/admin/system-credentials",
    {
      params: {
        header: adminHeaders,
        query: { limit: 100 },
      },
    },
    { enabled: hasSecret, retry: false }
  )
  const providersQuery = $api.useQuery(
    "get",
    "/v1/admin/llm-providers",
    { params: { header: adminHeaders } },
    { enabled: hasSecret, retry: false }
  )

  useEffect(() => {
    const error =
      integrationsQuery.error ?? credentialsQuery.error ?? providersQuery.error
    if (error) {
      toast.danger(errorMessage(error, "Failed to load admin data"))
    }
  }, [credentialsQuery.error, integrationsQuery.error, providersQuery.error])

  const createCredentialMutation = $api.useMutation(
    "post",
    "/v1/admin/system-credentials",
    {
      onSuccess: async () => {
        toast.success("System credential saved")
        setCredentialForm(emptyCredentialForm)
        await invalidateAdminQueries(queryClient)
      },
      onError: (error) => {
        toast.danger(errorMessage(error, "Failed to save credential"))
      },
    }
  )

  const revokeCredentialMutation = $api.useMutation(
    "delete",
    "/v1/admin/system-credentials/{id}",
    {
      onSuccess: async () => {
        toast.success("System credential revoked")
        await invalidateAdminQueries(queryClient)
      },
      onError: (error) => {
        toast.danger(errorMessage(error, "Failed to revoke credential"))
      },
    }
  )

  const integrations = integrationsQuery.data ?? EMPTY_INTEGRATIONS
  const credentials = (credentialsQuery.data?.data ??
    EMPTY_CREDENTIALS) as SystemCredential[]
  const providers = (providersQuery.data ?? EMPTY_PROVIDERS) as LLMProvider[]
  const adminFetching =
    integrationsQuery.isFetching ||
    credentialsQuery.isFetching ||
    providersQuery.isFetching
  const adminLoading =
    integrationsQuery.isLoading ||
    credentialsQuery.isLoading ||
    providersQuery.isLoading

  const filteredIntegrations = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return integrations
    return integrations.filter((item) =>
      [
        item.display_name,
        item.provider,
        item.nango_provider,
        item.unique_key,
        item.auth_mode,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(q)
    )
  }, [integrations, search])

  const filteredCredentials = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return credentials
    return credentials.filter((item) =>
      [item.label, item.provider_id, item.base_url, item.auth_scheme]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(q)
    )
  }, [credentials, search])

  function submitSecret(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextSecret = secretDraft.trim()
    if (!nextSecret) return
    setAdminSecret(nextSecret)
  }

  function submitCredential(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    createCredentialMutation.mutate({
      params: { header: adminHeaders },
      body: credentialForm,
    })
  }

  function clearSecret() {
    setAdminSecret("")
    setSecretDraft("")
    setSelectedIntegration(null)
    removeAdminQueries(queryClient)
  }

  return (
    <main className="fixed inset-0 flex overflow-hidden bg-background text-foreground">
      <AdminSidebar
        hasSecret={hasSecret}
        activeTab={activeTab}
        fetching={adminFetching}
        onRefresh={() => {
          void integrationsQuery.refetch()
          void credentialsQuery.refetch()
          void providersQuery.refetch()
        }}
        onClearSecret={clearSecret}
        onTabChange={setActiveTab}
      />

      <section className="min-h-0 flex-1 overflow-y-auto">
        {!hasSecret ? (
          <SecretGate
            value={secretDraft}
            onChange={setSecretDraft}
            onSubmit={submitSecret}
          />
        ) : (
          <div className="mx-auto flex w-full max-w-4xl flex-col gap-7 px-6 py-10">
            <AdminContentHeader
              activeTab={activeTab}
              search={search}
              onSearchChange={setSearch}
            />

            {activeTab === "integrations" ? (
              <IntegrationList
                loading={adminLoading}
                integrations={filteredIntegrations}
                onSelect={setSelectedIntegration}
              />
            ) : (
              <CredentialPanel
                credentials={filteredCredentials}
                providers={providers}
                form={credentialForm}
                saving={createCredentialMutation.isPending}
                revokingID={
                  revokeCredentialMutation.isPending
                    ? (revokeCredentialMutation.variables?.params.path.id ??
                      null)
                    : null
                }
                onFormChange={setCredentialForm}
                onSubmit={submitCredential}
                onRevoke={(id) =>
                  revokeCredentialMutation.mutate({
                    params: { header: adminHeaders, path: { id } },
                  })
                }
              />
            )}
          </div>
        )}
      </section>

      <IntegrationDialog
        key={selectedIntegration?.id ?? "closed"}
        adminSecret={adminSecret}
        definition={selectedIntegration}
        onOpenChange={(open) => {
          if (!open) setSelectedIntegration(null)
        }}
      />
    </main>
  )
}

async function invalidateAdminQueries(queryClient: QueryClient) {
  await Promise.all(
    Object.values(ADMIN_QUERY_KEYS).map((queryKey) =>
      queryClient.invalidateQueries({ queryKey })
    )
  )
}

function removeAdminQueries(queryClient: QueryClient) {
  for (const queryKey of Object.values(ADMIN_QUERY_KEYS)) {
    queryClient.removeQueries({ queryKey })
  }
}
