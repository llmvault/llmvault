"use client"

import { FormEvent, useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Modal, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import { TutorialVideo } from "@/components/tutorial-video"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import { clientConfig } from "@/lib/config/public-config"
import type { components } from "@/lib/api/schema"
import {
  type AvailableIntegration,
  integrationNeedsForm,
} from "@/app/w/(chat)/plugins/integration-auth"
import { IntegrationCredentialsForm } from "@/app/w/(chat)/plugins/integration-credentials-form"
import {
  type ConnectOptions,
  useConnectIntegration,
} from "@/app/w/(chat)/plugins/use-connect-integration"

type OnboardingStep = "team" | "connections" | "welcome" | "complete"
type Connection = components["schemas"]["connectionResponse"]

const STEPS: { id: Exclude<OnboardingStep, "complete">; label: string }[] = [
  { id: "team", label: "Create your team" },
  { id: "connections", label: "Connect your tools" },
  { id: "welcome", label: "Meet Hivy" },
]

export default function OnboardingPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { activeOrg, isLoading } = useAuth()
  const serverStep = (activeOrg?.onboarding_step ?? "complete") as OnboardingStep
  const [optimisticStep, setOptimisticStep] = useState<OnboardingStep | null>(null)
  const step = optimisticStep ?? serverStep
  const advance = $api.useMutation("patch", "/v1/orgs/current/onboarding")

  useEffect(() => {
    if (!isLoading && serverStep === "complete") router.replace("/w")
  }, [isLoading, router, serverStep])

  async function advanceTo(next: "welcome" | "complete") {
    try {
      await advance.mutateAsync({ body: { step: next } })
      setOptimisticStep(next)
      await queryClient.invalidateQueries({ queryKey: queryKeys.authMe() })
      if (next === "complete") router.replace("/w")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not continue onboarding"))
    }
  }

  if (isLoading || serverStep === "complete") return null

  return (
    <main className="min-h-screen bg-background text-foreground">
      <div className="mx-auto flex min-h-screen w-full max-w-5xl flex-col px-5 py-8 sm:px-8 sm:py-10">
        <header className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-foreground text-background">
              <AppIcon icon="bot" className="h-4.5 w-4.5" />
            </div>
            <span className="text-sm font-semibold">Hivy</span>
          </div>
          <span className="text-xs text-muted">About 2 minutes</span>
        </header>

        <div className="grid flex-1 items-start gap-10 py-12 md:grid-cols-[14rem_minmax(0,1fr)] md:py-20">
          <OnboardingProgress step={step} />
          <section className="min-w-0 md:max-w-2xl">
            {step === "team" ? <CreateTeamStep onCreated={() => setOptimisticStep("connections")} /> : null}
            {step === "connections" ? (
              <ConnectionsStep onContinue={() => void advanceTo("welcome")} advancing={advance.isPending} />
            ) : null}
            {step === "welcome" ? (
              <WelcomeStep onFinish={() => void advanceTo("complete")} finishing={advance.isPending} />
            ) : null}
          </section>
        </div>
      </div>
    </main>
  )
}

function OnboardingProgress({ step }: { step: OnboardingStep }) {
  const activeIndex = Math.max(0, STEPS.findIndex((item) => item.id === step))
  return (
    <ol className="flex gap-2 md:flex-col" aria-label="Onboarding progress">
      {STEPS.map((item, index) => {
        const current = index === activeIndex
        const complete = index < activeIndex
        return (
          <li key={item.id} className="flex min-w-0 flex-1 items-center gap-2.5 md:flex-none">
            <span
              className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-medium ${
                current
                  ? "bg-foreground text-background"
                  : complete
                    ? "bg-default text-foreground"
                    : "border border-border text-muted"
              }`}
              aria-current={current ? "step" : undefined}
            >
              {complete ? <AppIcon icon="check" className="h-3.5 w-3.5" /> : index + 1}
            </span>
            <span className={`hidden truncate text-sm md:block ${current ? "text-foreground" : "text-muted"}`}>
              {item.label}
            </span>
          </li>
        )
      })}
    </ol>
  )
}

function CreateTeamStep({ onCreated }: { onCreated: () => void }) {
  const queryClient = useQueryClient()
  const createTeam = $api.useMutation("post", "/v1/orgs/current/teams")
  const [name, setName] = useState("")

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const normalized = name.trim()
    if (!normalized || createTeam.isPending) return
    createTeam.mutate(
      { body: { name: normalized } },
      {
        onSuccess: async () => {
          await Promise.all([
            queryClient.invalidateQueries({ queryKey: queryKeys.teams() }),
            queryClient.invalidateQueries({ queryKey: queryKeys.authMe() }),
          ])
          onCreated()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not create team")),
      }
    )
  }

  return (
    <div className="flex flex-col gap-8">
      <div>
        <p className="text-sm font-medium text-primary">First, set your scope</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">Create your first team</h1>
        <p className="mt-3 max-w-xl text-sm leading-6 text-muted">
          Teams keep agents, channels, and permissions together. You can invite people and add more teams later.
        </p>
      </div>
      <form onSubmit={submit} className="flex max-w-lg flex-col gap-4">
        <label className="flex flex-col gap-2">
          <span className="text-sm font-medium">Team name</span>
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Engineering"
            autoComplete="organization"
            autoFocus
            disabled={createTeam.isPending}
          />
        </label>
        <div className="flex items-center justify-between gap-4 pt-2">
          <p className="text-xs text-muted">This step is required.</p>
          <Button type="submit" variant="primary" isDisabled={!name.trim() || createTeam.isPending}>
            {createTeam.isPending ? <Spinner color="current" size="sm" /> : null}
            Create team
            <AppIcon icon="arrow-right" className="h-4 w-4" />
          </Button>
        </div>
      </form>
    </div>
  )
}

function ConnectionsStep({ onContinue, advancing }: { onContinue: () => void; advancing: boolean }) {
  const integrationsQuery = $api.useQuery("get", "/v1/integrations/available")
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const integrations = integrationsQuery.data ?? []
  const connections = useMemo(
    () => connectionsQuery.data?.data ?? [],
    [connectionsQuery.data?.data]
  )
  const connectedProviders = useMemo(
    () => new Set(connections.flatMap((connection: Connection) => connection.provider ? [connection.provider] : [])),
    [connections]
  )
  const { connectIntegration, connectingId, isConnecting } = useConnectIntegration()
  const [formIntegration, setFormIntegration] = useState<AvailableIntegration | null>(null)

  function connect(integration: AvailableIntegration, options?: ConnectOptions) {
    if (!integration.id) return
    connectIntegration(integration.id, {
      ...options,
      installPlugins: true,
      onSuccess: () => {
        setFormIntegration(null)
        toast.success(`${integration.display_name ?? "Connection"} connected and plugin installed`)
        void connectionsQuery.refetch()
      },
    })
  }

  function requestConnect(integration: AvailableIntegration) {
    if (integrationNeedsForm(integration)) setFormIntegration(integration)
    else connect(integration)
  }

  return (
    <div className="flex flex-col gap-7">
      <div>
        <p className="text-sm font-medium text-primary">Bring your tools</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">Connect where work happens</h1>
        <p className="mt-3 max-w-xl text-sm leading-6 text-muted">
          Add any connections you want Hivy to use. Each connection automatically installs its matching plugin.
        </p>
      </div>

      <div className="overflow-hidden rounded-xl border border-border bg-surface">
        {integrationsQuery.isLoading ? (
          <div className="flex items-center gap-3 px-4 py-5 text-sm text-muted">
            <Spinner size="sm" /> Loading connections
          </div>
        ) : integrations.length === 0 ? (
          <p className="px-4 py-5 text-sm text-muted">No connections are available yet.</p>
        ) : (
          integrations.map((integration, index) => {
            const provider = integration.provider ?? ""
            const connected = connectedProviders.has(provider)
            const connecting = connectingId === integration.id
            return (
              <div
                key={integration.id ?? provider}
                className={`flex items-center gap-3 px-4 py-3.5 ${index ? "border-t border-border" : ""}`}
              >
                <IntegrationLogo provider={provider} size={36} className="rounded-lg" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{integration.display_name || provider}</p>
                  <p className="mt-0.5 text-xs text-muted">
                    {connected ? "Connected, plugin installed" : "Connect and install the matching plugin"}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant={connected ? "tertiary" : "primary"}
                  isDisabled={connected || isConnecting}
                  onPress={() => requestConnect(integration)}
                >
                  {connecting ? <Spinner color="current" size="sm" /> : connected ? <AppIcon icon="check" className="h-4 w-4" /> : null}
                  {connected ? "Connected" : "Connect"}
                </Button>
              </div>
            )
          })
        )}
      </div>

      <div className="flex items-center justify-between gap-4">
        <p className="text-xs text-muted">You can add or remove connections later.</p>
        <Button variant="primary" onPress={onContinue} isDisabled={advancing || isConnecting}>
          {advancing ? <Spinner color="current" size="sm" /> : null}
          {connections.length ? "Continue" : "Skip for now"}
          <AppIcon icon="arrow-right" className="h-4 w-4" />
        </Button>
      </div>

      <Modal isOpen={formIntegration !== null} onOpenChange={(open) => !open && setFormIntegration(null)}>
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

function WelcomeStep({ onFinish, finishing }: { onFinish: () => void; finishing: boolean }) {
  const videoURL = clientConfig().tutorialVideos.welcome ?? ""
  return (
    <div className="flex flex-col gap-7">
      <div>
        <p className="text-sm font-medium text-primary">You are ready</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">See Hivy in action</h1>
        <p className="mt-3 max-w-xl text-sm leading-6 text-muted">
          This 30-second tour shows how to start a conversation, choose a team, and put your new connections to work.
        </p>
      </div>

      {videoURL ? (
        <TutorialVideo url={videoURL} title="Welcome to Hivy" />
      ) : (
        <div className="flex aspect-video w-full items-center justify-center rounded-xl border border-border bg-surface-secondary">
          <div className="text-center">
            <AppIcon icon="video" className="mx-auto h-7 w-7 text-muted" />
            <p className="mt-2 text-sm font-medium">Welcome video coming soon</p>
            <p className="mt-1 text-xs text-muted">Set HIVY_WELCOME_VIDEO_URL to publish it.</p>
          </div>
        </div>
      )}

      <div className="flex justify-end">
        <Button variant="primary" onPress={onFinish} isDisabled={finishing}>
          {finishing ? <Spinner color="current" size="sm" /> : null}
          Go to Hivy
          <AppIcon icon="arrow-right" className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
