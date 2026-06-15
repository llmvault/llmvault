"use client"

import { useEffect, useMemo, useState, type FormEvent } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import {
  adminDataQueryKey,
  createSystemCredential,
  errorMessage,
  loadAdminData,
  revokeSystemCredential,
} from "./admin-api"
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
  const [adminSecretVersion, setAdminSecretVersion] = useState(0)
  const [activeTab, setActiveTab] = useState<AdminTab>("integrations")
  const [search, setSearch] = useState("")
  const [selectedIntegration, setSelectedIntegration] =
    useState<AdminIntegrationDefinition | null>(null)
  const [credentialForm, setCredentialForm] =
    useState<CredentialForm>(emptyCredentialForm)

  const hasSecret = adminSecret.trim().length > 0
  const adminQuery = useQuery({
    queryKey: adminDataQueryKey(adminSecretVersion),
    queryFn: () => loadAdminData(adminSecret),
    enabled: hasSecret,
    retry: false,
  })

  useEffect(() => {
    if (adminQuery.error) {
      toast.danger(errorMessage(adminQuery.error, "Failed to load admin data"))
    }
  }, [adminQuery.error])

  const createCredentialMutation = useMutation({
    mutationFn: (body: CredentialForm) =>
      createSystemCredential(adminSecret, body),
    onSuccess: async () => {
      toast.success("System credential saved")
      setCredentialForm(emptyCredentialForm)
      await queryClient.invalidateQueries({
        queryKey: adminDataQueryKey(adminSecretVersion),
      })
    },
    onError: (error) => {
      toast.danger(errorMessage(error, "Failed to save credential"))
    },
  })

  const revokeCredentialMutation = useMutation({
    mutationFn: (id: string) => revokeSystemCredential(adminSecret, id),
    onSuccess: async () => {
      toast.success("System credential revoked")
      await queryClient.invalidateQueries({
        queryKey: adminDataQueryKey(adminSecretVersion),
      })
    },
    onError: (error) => {
      toast.danger(errorMessage(error, "Failed to revoke credential"))
    },
  })

  const integrations = adminQuery.data?.integrations ?? EMPTY_INTEGRATIONS
  const credentials = adminQuery.data?.credentials ?? EMPTY_CREDENTIALS
  const providers = adminQuery.data?.providers ?? EMPTY_PROVIDERS

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
    setAdminSecretVersion((version) => version + 1)
  }

  function submitCredential(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    createCredentialMutation.mutate(credentialForm)
  }

  function clearSecret() {
    setAdminSecret("")
    setSecretDraft("")
    setSelectedIntegration(null)
    queryClient.removeQueries({ queryKey: ["admin", "setup"] })
  }

  return (
    <main className="fixed inset-0 flex overflow-hidden bg-background text-foreground">
      <AdminSidebar
        hasSecret={hasSecret}
        activeTab={activeTab}
        fetching={adminQuery.isFetching}
        onRefresh={() => void adminQuery.refetch()}
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
                loading={adminQuery.isLoading}
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
                    ? (revokeCredentialMutation.variables ?? null)
                    : null
                }
                onFormChange={setCredentialForm}
                onSubmit={submitCredential}
                onRevoke={(id) => revokeCredentialMutation.mutate(id)}
              />
            )}
          </div>
        )}
      </section>

      <IntegrationDialog
        key={selectedIntegration?.id ?? "closed"}
        adminSecret={adminSecret}
        adminSecretVersion={adminSecretVersion}
        definition={selectedIntegration}
        onOpenChange={(open) => {
          if (!open) setSelectedIntegration(null)
        }}
      />
    </main>
  )
}
