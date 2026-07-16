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
import {
  emptyCredentialForm,
  type CredentialForm,
  type LLMProvider,
  type SystemCredential,
} from "./types"

const EMPTY_CREDENTIALS: SystemCredential[] = []
const EMPTY_PROVIDERS: LLMProvider[] = []

export function AdminClient() {
  const queryClient = useQueryClient()
  const [secretDraft, setSecretDraft] = useState("")
  const [adminSecret, setAdminSecret] = useState("")
  const [search, setSearch] = useState("")
  const [credentialForm, setCredentialForm] =
    useState<CredentialForm>(emptyCredentialForm)

  const hasSecret = adminSecret.trim().length > 0
  const adminHeaders = useMemo(
    () => adminSecretHeader(adminSecret),
    [adminSecret]
  )
  const credentialsQuery = $api.useQuery(
    "get",
    "/v1/admin/system-credentials",
    { params: { header: adminHeaders, query: { limit: 100 } } },
    { enabled: hasSecret, retry: false }
  )
  const providersQuery = $api.useQuery(
    "get",
    "/v1/admin/llm-providers",
    { params: { header: adminHeaders } },
    { enabled: hasSecret, retry: false }
  )

  useEffect(() => {
    const error = credentialsQuery.error ?? providersQuery.error
    if (error) toast.danger(errorMessage(error, "Failed to load admin data"))
  }, [credentialsQuery.error, providersQuery.error])

  const createCredentialMutation = $api.useMutation(
    "post",
    "/v1/admin/system-credentials",
    {
      onSuccess: async () => {
        toast.success("System credential saved")
        setCredentialForm(emptyCredentialForm)
        await invalidateAdminQueries(queryClient)
      },
      onError: (error) =>
        toast.danger(errorMessage(error, "Failed to save credential")),
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
      onError: (error) =>
        toast.danger(errorMessage(error, "Failed to revoke credential")),
    }
  )

  const credentials = (credentialsQuery.data?.data ??
    EMPTY_CREDENTIALS) as SystemCredential[]
  const providers = (providersQuery.data ?? EMPTY_PROVIDERS) as LLMProvider[]
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
    if (nextSecret) setAdminSecret(nextSecret)
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
    removeAdminQueries(queryClient)
  }

  return (
    <main className="fixed inset-0 flex overflow-hidden bg-background text-foreground">
      <AdminSidebar
        hasSecret={hasSecret}
        fetching={credentialsQuery.isFetching || providersQuery.isFetching}
        onRefresh={() => {
          void credentialsQuery.refetch()
          void providersQuery.refetch()
        }}
        onClearSecret={clearSecret}
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
            <AdminContentHeader search={search} onSearchChange={setSearch} />
            <CredentialPanel
              credentials={filteredCredentials}
              providers={providers}
              form={credentialForm}
              saving={createCredentialMutation.isPending}
              revokingID={
                revokeCredentialMutation.isPending
                  ? (revokeCredentialMutation.variables?.params.path.id ?? null)
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
          </div>
        )}
      </section>
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
