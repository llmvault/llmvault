"use client"

import posthog from "posthog-js"
import { FormEvent, useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Modal, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import { LogoMark } from "@/components/logo"
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
type VisibleStep = Exclude<OnboardingStep, "complete">
type ProgressStep = Exclude<VisibleStep, "welcome">
type Connection = components["schemas"]["connectionResponse"]

const PROGRESS_STEPS: {
  id: ProgressStep
  label: string
  description: string
}[] = [
  {
    id: "team",
    label: "Create your team",
    description: "Control agent and teammate access.",
  },
  {
    id: "connections",
    label: "Install plugins",
    description:
      "Give your agents restricted access to the tools your team uses",
  },
]

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

export default function OnboardingPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { activeOrg, isLoading } = useAuth()
  const serverStep = (activeOrg?.onboarding_step ??
    "complete") as OnboardingStep
  const [optimisticStep, setOptimisticStep] = useState<OnboardingStep | null>(
    null
  )
  const step = optimisticStep ?? serverStep
  const advance = $api.useMutation("patch", "/v1/orgs/current/onboarding")

  useEffect(() => {
    if (!isLoading && serverStep === "complete") router.replace("/w")
  }, [isLoading, router, serverStep])

  useEffect(() => {
    if (!isLoading && serverStep !== "complete") {
      posthog.capture("onboarding_viewed", { step: serverStep })
    }
  }, [isLoading, serverStep])

  async function advanceTo(next: "welcome" | "complete") {
    try {
      await advance.mutateAsync({ body: { step: next } })
      setOptimisticStep(next)
      await queryClient.invalidateQueries({ queryKey: queryKeys.authMe() })
      if (next === "complete") {
        posthog.capture("onboarding_completed")
        router.replace("/w")
      }
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not continue onboarding"))
    }
  }

  if (isLoading || serverStep === "complete" || step === "complete") return null

  return (
    <main className="min-h-dvh bg-surface-secondary p-0 text-foreground md:p-5">
      <div className="mx-auto grid min-h-dvh w-full max-w-7xl overflow-hidden border-border bg-background shadow-sm md:h-[calc(100dvh-2.5rem)] md:min-h-0 md:rounded-3xl md:border lg:grid-cols-[22rem_minmax(0,1fr)]">
        <SetupSidebar step={step} />

        <div className="flex min-h-0 min-w-0 flex-col">
          <MobileSetupHeader step={step} />
          <header className="hidden h-16 shrink-0 items-center justify-between border-b border-border px-8 lg:flex">
            <p className="text-sm font-medium">Set up Hivy</p>
            <p className="text-xs text-muted">
              {step === "welcome"
                ? "Setup complete"
                : `Step ${progressIndex(step) + 1} of ${PROGRESS_STEPS.length}`}
            </p>
          </header>

          <section
            key={step}
            className="flex min-h-0 flex-1 flex-col overflow-y-auto px-4 py-7 sm:px-8 sm:py-12 lg:px-12"
          >
            {step === "team" ? (
              <CreateTeamStep
                onCreated={() => setOptimisticStep("connections")}
              />
            ) : null}
            {step === "connections" ? (
              <ConnectionsStep
                onContinue={() => void advanceTo("welcome")}
                advancing={advance.isPending}
              />
            ) : null}
            {step === "welcome" ? (
              <WelcomeStep
                onFinish={() => void advanceTo("complete")}
                finishing={advance.isPending}
              />
            ) : null}
          </section>
        </div>
      </div>
    </main>
  )
}

function progressIndex(step: VisibleStep) {
  if (step === "welcome") return PROGRESS_STEPS.length
  return Math.max(
    0,
    PROGRESS_STEPS.findIndex((item) => item.id === step)
  )
}

function SetupSidebar({ step }: { step: VisibleStep }) {
  const activeIndex = progressIndex(step)
  return (
    <aside className="hidden flex-col border-r border-border bg-surface px-6 py-7 lg:flex">
      <div className="flex items-center gap-2.5">
        <LogoMark className="h-9 w-9 shrink-0" />
        <div>
          <p className="text-sm font-semibold">Hivy</p>
          <p className="text-xs text-muted">Workspace setup</p>
        </div>
      </div>

      <ol className="mt-14 space-y-1" aria-label="Onboarding progress">
        {PROGRESS_STEPS.map((item, index) => {
          const current = index === activeIndex
          const complete = index < activeIndex
          return (
            <li
              key={item.id}
              className={`relative flex gap-3 rounded-xl px-3 py-3.5 ${
                current ? "bg-default" : ""
              }`}
              aria-current={current ? "step" : undefined}
            >
              <span
                className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold ${
                  current
                    ? "bg-primary text-primary-foreground"
                    : complete
                      ? "bg-foreground text-background"
                      : "border border-border text-muted"
                }`}
              >
                {complete ? (
                  <AppIcon icon="check" className="h-3.5 w-3.5" />
                ) : (
                  index + 1
                )}
              </span>
              <span className="min-w-0">
                <span
                  className={`block text-sm font-medium ${current ? "text-foreground" : "text-muted"}`}
                >
                  {item.label}
                </span>
                <span className="mt-1 block text-xs leading-4 text-muted">
                  {item.description}
                </span>
              </span>
            </li>
          )
        })}
      </ol>

      <div className="mt-auto border-t border-border pt-5">
        <div className="flex items-start gap-2.5 text-xs leading-5 text-muted">
          <AppIcon icon="clock" className="mt-0.5 h-4 w-4 shrink-0" />
          <p>
            Usually takes less than two minutes. You can change everything
            later.
          </p>
        </div>
      </div>
    </aside>
  )
}

function MobileSetupHeader({ step }: { step: VisibleStep }) {
  const activeIndex = progressIndex(step)
  return (
    <header className="border-b border-border bg-surface px-5 py-4 lg:hidden">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <LogoMark className="h-8 w-8 shrink-0" />
          <span className="text-sm font-semibold">Set up Hivy</span>
        </div>
        <span className="text-xs text-muted">
          {step === "welcome"
            ? "Complete"
            : `${activeIndex + 1} of ${PROGRESS_STEPS.length}`}
        </span>
      </div>
      <div
        className="mt-4 grid grid-cols-2 gap-1.5"
        aria-label="Onboarding progress"
      >
        {PROGRESS_STEPS.map((item, index) => (
          <span
            key={item.id}
            className={`h-1 rounded-full ${index <= activeIndex ? "bg-primary" : "bg-default"}`}
          />
        ))}
      </div>
    </header>
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
          posthog.capture("onboarding_team_created")
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
    <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col justify-center py-2">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          Create your first team
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted">
          Teams control which agents and teammates can access plugins and
          knowledge sources.
        </p>
      </div>

      <form onSubmit={submit} className="mt-8 w-full">
        <Input
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="Team name"
          aria-label="Team name"
          autoComplete="organization"
          autoFocus
          disabled={createTeam.isPending}
          className="w-full"
        />

        <div className="mt-4 flex justify-end">
          <Button
            className="w-full sm:w-auto"
            type="submit"
            variant="primary"
            isDisabled={!name.trim() || createTeam.isPending}
          >
            {createTeam.isPending ? (
              <Spinner color="current" size="sm" />
            ) : null}
            Create team
            <AppIcon icon="arrow-right" className="h-4 w-4" />
          </Button>
        </div>
      </form>
    </div>
  )
}

function ConnectionsStep({
  onContinue,
  advancing,
}: {
  onContinue: () => void
  advancing: boolean
}) {
  const integrationsQuery = $api.useQuery("get", "/v1/integrations/supported")
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
        posthog.capture("onboarding_plugin_connected", {
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
            Install plugins
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
              description="Your workspace administrator still needs to configure the connection catalog. You can continue and add tools later."
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
              const configured = integration.configured ?? false
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
                        : configured
                          ? (CONNECTION_DESCRIPTIONS[provider] ??
                            "Connect this tool to use it with Hivy.")
                          : "Supported by Hivy, awaiting workspace configuration"}
                    </p>
                  </div>
                  <Button
                    className="col-start-2 justify-self-start sm:col-auto sm:justify-self-auto"
                    size="sm"
                    variant={connected ? "tertiary" : "secondary"}
                    isDisabled={connected || isConnecting || !configured}
                    onPress={() => requestConnect(integration)}
                  >
                    {connecting ? <Spinner color="current" size="sm" /> : null}
                    {connected ? (
                      <>
                        <AppIcon icon="check" className="h-4 w-4" /> Ready
                      </>
                    ) : configured ? (
                      "Connect"
                    ) : (
                      "Unavailable"
                    )}
                  </Button>
                </div>
              )
            })
          )}
        </div>
      </div>

      <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        {connections.length === 0 ? (
          <p className="text-xs text-muted">
            Connect at least one plugin to continue.
          </p>
        ) : null}
        <Button
          className="w-full sm:ml-auto sm:w-auto"
          variant="primary"
          onPress={onContinue}
          isDisabled={connections.length === 0 || advancing || isConnecting}
        >
          {advancing ? <Spinner color="current" size="sm" /> : null}
          Continue
          <AppIcon icon="arrow-right" className="h-4 w-4" />
        </Button>
      </div>

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

function WelcomeStep({
  onFinish,
  finishing,
}: {
  onFinish: () => void
  finishing: boolean
}) {
  const videoURL = clientConfig().tutorialVideos.welcome ?? ""
  return (
    <div className="mx-auto flex w-full max-w-4xl flex-1 flex-col justify-center py-2">
      <div className="text-center">
        <div className="mx-auto flex h-11 w-11 items-center justify-center rounded-2xl bg-success/15 text-success">
          <AppIcon icon="check" className="h-5 w-5" />
        </div>
        <p className="text-primary mt-5 text-sm font-semibold">
          Setup complete
        </p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight sm:text-4xl">
          Your team is ready to work
        </h1>
        <p className="mx-auto mt-4 max-w-2xl text-base leading-7 text-muted">
          Take a 30-second look at the product, or jump straight into your first
          conversation.
        </p>
      </div>

      <div className="mt-8 grid overflow-hidden rounded-2xl border border-border bg-surface xl:grid-cols-[minmax(0,1.55fr)_minmax(15rem,0.75fr)]">
        <div className="min-w-0 bg-surface-secondary p-3">
          {videoURL ? (
            <TutorialVideo url={videoURL} title="Welcome to Hivy" />
          ) : (
            <ProductPreview />
          )}
        </div>
        <div className="flex flex-col border-t border-border p-5 xl:border-t-0 xl:border-l">
          <p className="text-xs font-semibold tracking-wider text-muted uppercase">
            You can start with
          </p>
          <ul className="mt-4 space-y-4">
            <WelcomeAction
              icon="message-square"
              text="Ask Hivy to plan or complete a task"
            />
            <WelcomeAction icon="plug" text="Use the tools you connected" />
            <WelcomeAction
              icon="user-plus"
              text="Invite teammates when you are ready"
            />
          </ul>
          <p className="mt-auto pt-6 text-xs leading-5 text-muted">
            Tutorials stay available inside each feature, so there is nothing to
            memorize now.
          </p>
        </div>
      </div>

      <div className="mt-7 flex justify-center">
        <Button variant="primary" onPress={onFinish} isDisabled={finishing}>
          {finishing ? <Spinner color="current" size="sm" /> : null}
          Start your first chat
          <AppIcon icon="arrow-right" className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}

function ProductPreview() {
  return (
    <div className="flex aspect-video min-h-56 w-full flex-col overflow-hidden rounded-xl border border-border bg-background shadow-sm sm:min-h-64">
      <div className="flex h-10 shrink-0 items-center gap-1.5 border-b border-border px-4">
        <span className="h-2 w-2 rounded-full bg-muted/40" />
        <span className="h-2 w-2 rounded-full bg-muted/25" />
        <span className="h-2 w-2 rounded-full bg-muted/15" />
        <span className="ml-3 text-[10px] font-medium text-muted">
          Your first conversation
        </span>
      </div>
      <div className="flex flex-1 flex-col justify-center gap-4 p-5 sm:p-7">
        <div className="bg-primary text-primary-foreground ml-auto max-w-[78%] rounded-2xl rounded-br-md px-4 py-2.5 text-xs leading-5">
          Help me plan the launch for next Friday.
        </div>
        <div className="flex max-w-[88%] items-start gap-2.5">
          <LogoMark className="h-7 w-7 shrink-0" />
          <div className="rounded-2xl rounded-tl-md bg-default px-4 py-3 text-xs leading-5 text-foreground">
            I’ll turn that into a clear plan, then we can assign the work and
            track progress together.
          </div>
        </div>
      </div>
      <div className="m-3 flex h-10 items-center rounded-xl border border-border bg-surface px-3 text-[10px] text-muted">
        Ask Hivy anything...
        <span className="ml-auto flex h-6 w-6 items-center justify-center rounded-lg bg-foreground text-background">
          <AppIcon icon="arrow-right" className="h-3 w-3" />
        </span>
      </div>
    </div>
  )
}

function WelcomeAction({ icon, text }: { icon: string; text: string }) {
  return (
    <li className="flex min-w-0 items-start gap-3 text-sm leading-5">
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-default text-muted">
        <AppIcon icon={icon} className="h-3.5 w-3.5" />
      </span>
      <span className="min-w-0 pt-1">{text}</span>
    </li>
  )
}
