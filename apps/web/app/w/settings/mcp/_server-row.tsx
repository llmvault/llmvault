"use client"

import { type FormEvent, useState } from "react"
import { Button, Input, Label, Spinner, Switch, TextField } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
import type { McpAuthorizationInput, McpServer } from "./_lib/mcp-api"
import {
  assignmentSummary,
  authLabel,
  authorizationPolicyLabel,
  authorizationPrincipal,
  healthLabel,
  safeServerHost,
  transportLabel,
} from "./_lib/mcp-ui"

export interface AccessOption {
  id: string
  name: string
  description?: string
}

const HEALTH_META = {
  unknown: { icon: "circle", className: "text-muted-foreground" },
  checking: { icon: "loader-circle", className: "text-primary" },
  healthy: { icon: "circle-check", className: "text-success" },
  degraded: { icon: "circle-alert", className: "text-warning" },
  unhealthy: { icon: "circle-alert", className: "text-danger" },
} as const

export function ServerRow({
  server,
  teams,
  agents,
  isAdmin,
  isBusy,
  accessLoading,
  onTest,
  onOAuth,
  onConfigureAuth,
  onToggleTeam,
  onToggleAgent,
  onRemove,
}: {
  server: McpServer
  teams: AccessOption[]
  agents: AccessOption[]
  isAdmin: boolean
  isBusy: boolean
  accessLoading: boolean
  onTest: () => void
  onOAuth: () => void
  onConfigureAuth: (input: McpAuthorizationInput) => Promise<void>
  onToggleTeam: (teamID: string, enabled: boolean) => void
  onToggleAgent: (agentID: string, enabled: boolean) => void
  onRemove: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [editingCredential, setEditingCredential] = useState(false)
  const health = HEALTH_META[server.healthStatus]
  const canManageDefinition = server.scope === "personal" || isAdmin
  const canConfigureCredential =
    server.scope === "personal" ||
    server.authorizationPolicy === "user_required" ||
    server.authorizationPolicy === "prefer_user" ||
    isAdmin
  const canConnectOAuth =
    server.authType === "oauth_authorization_code" &&
    (server.scope === "personal" ||
      server.authorizationPolicy === "user_required" ||
      server.authorizationPolicy === "prefer_user" ||
      isAdmin)

  return (
    <article className="overflow-hidden rounded-2xl border border-border bg-surface">
      <div className="flex items-start gap-3.5 px-4 py-4">
        <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
          <AppIcon icon="plug-zap" className="h-5 w-5 text-muted" />
        </span>

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h2 className="truncate text-sm font-medium text-foreground">
              {server.name}
            </h2>
            <ScopeBadge scope={server.scope} />
          </div>
          <p className="text-muted-foreground mt-0.5 truncate text-xs">
            {safeServerHost(server.url)}
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs">
            <span
              className={cn(
                "inline-flex items-center gap-1.5",
                health.className
              )}
            >
              <AppIcon
                icon={health.icon}
                className={cn(
                  "h-3.5 w-3.5",
                  server.healthStatus === "checking" && "animate-spin"
                )}
              />
              {healthLabel(server.healthStatus)}
            </span>
            <span className="text-muted-foreground inline-flex items-center gap-1.5">
              <AppIcon icon="key-round" className="h-3.5 w-3.5" />
              {server.scope === "org" &&
              server.authorizationPolicy === "service_required" &&
              !isAdmin
                ? "Organization credential"
                : authLabel(
                    server.authType,
                    server.authStatus,
                    server.secretSet
                  )}
            </span>
            {server.authType !== "none" ? (
              <span className="text-muted-foreground">
                {authorizationPolicyLabel(server.authorizationPolicy)}
              </span>
            ) : null}
            <span className="text-muted-foreground">
              {assignmentSummary(server)}
            </span>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          {canConnectOAuth && server.authStatus !== "connected" ? (
            <Button
              size="sm"
              variant="secondary"
              isDisabled={isBusy}
              onPress={onOAuth}
            >
              {server.authStatus === "expired" ? "Reconnect" : "Connect"}
            </Button>
          ) : null}
          <Button
            size="sm"
            variant="tertiary"
            aria-expanded={expanded}
            aria-controls={`mcp-server-${server.id}-details`}
            onPress={() => setExpanded((value) => !value)}
          >
            Manage
            <AppIcon
              icon="chevron-down"
              className={cn(
                "h-3.5 w-3.5 transition-transform",
                expanded && "rotate-180"
              )}
            />
          </Button>
        </div>
      </div>

      {expanded ? (
        <div
          id={`mcp-server-${server.id}-details`}
          className="bg-card border-t border-border px-4 py-4"
        >
          <div className="flex flex-col gap-6">
            <section className="flex flex-col gap-3">
              <SectionHeading
                title="Connection"
                description="Hivy checks this endpoint before agents use it."
              />
              <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[7rem_1fr]">
                <dt className="text-muted-foreground">Endpoint</dt>
                <dd className="min-w-0 break-all text-foreground">
                  {server.url}
                </dd>
                <dt className="text-muted-foreground">Transport</dt>
                <dd className="text-foreground">
                  {transportLabel(server.transport)}
                </dd>
                {server.toolCount !== null ? (
                  <>
                    <dt className="text-muted-foreground">Tools found</dt>
                    <dd className="text-foreground">{server.toolCount}</dd>
                  </>
                ) : null}
              </dl>
              <div>
                <Button
                  size="sm"
                  variant="secondary"
                  isDisabled={isBusy}
                  onPress={onTest}
                >
                  {isBusy ? <Spinner size="sm" color="current" /> : null}
                  Test connection
                </Button>
              </div>
            </section>

            {server.authType !== "none" ? (
              <section className="flex flex-col gap-3 border-t border-border pt-5">
                <SectionHeading
                  title="Authentication"
                  description={
                    server.authorizationLabel
                      ? `Connected as ${server.authorizationLabel}. Secret values are never displayed.`
                      : "Secret values are encrypted and never displayed after you save them."
                  }
                />
                {canConnectOAuth ? (
                  <div>
                    <Button
                      size="sm"
                      variant="secondary"
                      isDisabled={isBusy}
                      onPress={onOAuth}
                    >
                      {server.authStatus === "connected"
                        ? "Reconnect account"
                        : "Connect account"}
                    </Button>
                  </div>
                ) : canConfigureCredential ? (
                  editingCredential ? (
                    <CredentialEditor
                      server={server}
                      isPending={isBusy}
                      onCancel={() => setEditingCredential(false)}
                      onSave={async (input) => {
                        await onConfigureAuth(input)
                        setEditingCredential(false)
                      }}
                    />
                  ) : (
                    <div>
                      <Button
                        size="sm"
                        variant="secondary"
                        isDisabled={isBusy}
                        onPress={() => setEditingCredential(true)}
                      >
                        {server.secretSet
                          ? "Replace credential"
                          : "Add credential"}
                      </Button>
                    </div>
                  )
                ) : (
                  <p className="text-muted-foreground text-sm">
                    A workspace admin manages this organization credential.
                  </p>
                )}
              </section>
            ) : null}

            <AccessSection
              server={server}
              teams={teams}
              agents={agents}
              isAdmin={isAdmin}
              isBusy={isBusy}
              isLoading={accessLoading}
              onToggleTeam={onToggleTeam}
              onToggleAgent={onToggleAgent}
            />

            {canManageDefinition ? (
              <section className="flex items-center justify-between gap-4 border-t border-border pt-5">
                <div>
                  <h3 className="text-sm font-medium text-foreground">
                    Remove server
                  </h3>
                  <p className="text-muted-foreground mt-0.5 text-xs">
                    Revokes access and removes its saved authorization.
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="danger-soft"
                  isDisabled={isBusy}
                  onPress={onRemove}
                >
                  Remove
                </Button>
              </section>
            ) : null}
          </div>
        </div>
      ) : null}
    </article>
  )
}

function AccessSection({
  server,
  teams,
  agents,
  isAdmin,
  isBusy,
  isLoading,
  onToggleTeam,
  onToggleAgent,
}: {
  server: McpServer
  teams: AccessOption[]
  agents: AccessOption[]
  isAdmin: boolean
  isBusy: boolean
  isLoading: boolean
  onToggleTeam: (teamID: string, enabled: boolean) => void
  onToggleAgent: (agentID: string, enabled: boolean) => void
}) {
  const canManageAccess = server.scope === "personal" || isAdmin
  return (
    <section className="flex flex-col gap-4 border-t border-border pt-5">
      <SectionHeading
        title="Access"
        description={
          server.scope === "personal"
            ? "Choose which agents can use this server when you start a chat or schedule."
            : "Team grants are inherited by every agent in that team. Direct grants apply to one agent."
        }
      />
      {!canManageAccess ? (
        <p className="text-muted-foreground text-sm">
          A workspace admin manages access for this organization server.
        </p>
      ) : isLoading ? (
        <div className="text-muted-foreground flex items-center gap-2 text-sm">
          <Spinner size="sm" /> Loading access…
        </div>
      ) : (
        <div className="grid gap-5 sm:grid-cols-2">
          {server.scope === "org" ? (
            <AccessList
              title="Teams"
              empty="No teams are available."
              options={teams}
              selectedIds={server.teamIds}
              disabled={isBusy}
              onToggle={onToggleTeam}
            />
          ) : null}
          <AccessList
            title={
              server.scope === "personal" ? "Agents" : "Direct agent access"
            }
            empty="No agents are available."
            options={agents}
            selectedIds={server.agentIds}
            disabled={isBusy}
            onToggle={onToggleAgent}
          />
        </div>
      )}
    </section>
  )
}

function AccessList({
  title,
  empty,
  options,
  selectedIds,
  disabled,
  onToggle,
}: {
  title: string
  empty: string
  options: AccessOption[]
  selectedIds: string[]
  disabled: boolean
  onToggle: (id: string, enabled: boolean) => void
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <h4 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
        {title}
      </h4>
      {options.length === 0 ? (
        <p className="text-muted-foreground text-sm">{empty}</p>
      ) : (
        <div className="max-h-56 overflow-y-auto rounded-xl border border-border bg-surface">
          {options.map((option, index) => {
            const selected = selectedIds.includes(option.id)
            return (
              <div
                key={option.id}
                className={cn(
                  "flex items-center gap-3 px-3 py-2.5",
                  index < options.length - 1 && "border-b border-border"
                )}
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm text-foreground">
                    {option.name}
                  </p>
                  {option.description ? (
                    <p className="text-muted-foreground truncate text-xs">
                      {option.description}
                    </p>
                  ) : null}
                </div>
                <Switch
                  aria-label={`${selected ? "Remove" : "Grant"} ${option.name}`}
                  isSelected={selected}
                  isDisabled={disabled}
                  onChange={(enabled) => onToggle(option.id, enabled)}
                >
                  <Switch.Control>
                    <Switch.Thumb />
                  </Switch.Control>
                </Switch>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function CredentialEditor({
  server,
  isPending,
  onCancel,
  onSave,
}: {
  server: McpServer
  isPending: boolean
  onCancel: () => void
  onSave: (input: McpAuthorizationInput) => Promise<void>
}) {
  const [bearerToken, setBearerToken] = useState("")
  const [headerValue, setHeaderValue] = useState("")
  const [clientId, setClientId] = useState("")
  const [clientSecret, setClientSecret] = useState("")
  const [scopes, setScopes] = useState("")
  const principalType = authorizationPrincipal(
    server.scope,
    server.authorizationPolicy
  )
  const valid =
    server.authType === "static_bearer"
      ? bearerToken.trim() !== ""
      : server.authType === "static_header"
        ? headerValue.trim() !== ""
        : clientId.trim() !== "" && clientSecret.trim() !== ""

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!valid) return
    const input: McpAuthorizationInput =
      server.authType === "static_bearer"
        ? { principal_type: principalType, bearer_token: bearerToken.trim() }
        : server.authType === "static_header"
          ? { principal_type: principalType, header_value: headerValue.trim() }
          : {
              principal_type: principalType,
              client_id: clientId.trim(),
              client_secret: clientSecret.trim(),
              scopes: scopes
                .split(/[ ,]+/)
                .map((value) => value.trim())
                .filter(Boolean),
            }
    setBearerToken("")
    setHeaderValue("")
    setClientSecret("")
    await onSave(input)
  }

  return (
    <form onSubmit={save} className="flex flex-col gap-3">
      {server.authType === "static_bearer" ? (
        <SecretInput
          label="New bearer token"
          value={bearerToken}
          onChange={setBearerToken}
        />
      ) : server.authType === "static_header" ? (
        <SecretInput
          label={`New ${server.headerName || "header"} value`}
          value={headerValue}
          onChange={setHeaderValue}
        />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          <TextInput
            label="Client ID"
            value={clientId}
            onChange={setClientId}
          />
          <SecretInput
            label="Client secret"
            value={clientSecret}
            onChange={setClientSecret}
          />
          <TextInput label="Scopes" value={scopes} onChange={setScopes} />
        </div>
      )}
      <div className="flex justify-end gap-2">
        <Button
          size="sm"
          variant="tertiary"
          isDisabled={isPending}
          onPress={onCancel}
        >
          Cancel
        </Button>
        <Button
          size="sm"
          variant="primary"
          type="submit"
          isDisabled={!valid || isPending}
        >
          {isPending ? <Spinner size="sm" color="current" /> : null}
          Save credential
        </Button>
      </div>
    </form>
  )
}

function TextInput({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <TextField className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        autoComplete="off"
        className="h-9 w-full rounded-md text-sm"
      />
    </TextField>
  )
}

function SecretInput({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <TextField className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      <Input
        type="password"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        autoComplete="new-password"
        className="h-9 w-full rounded-md text-sm"
      />
    </TextField>
  )
}

function ScopeBadge({ scope }: { scope: McpServer["scope"] }) {
  return (
    <span className="text-muted-foreground rounded-md bg-default px-1.5 py-0.5 text-[11px] font-medium">
      {scope === "personal" ? "Personal" : "Organization"}
    </span>
  )
}

function SectionHeading({
  title,
  description,
}: {
  title: string
  description: string
}) {
  return (
    <div>
      <h3 className="text-sm font-medium text-foreground">{title}</h3>
      <p className="text-muted-foreground mt-0.5 text-xs">{description}</p>
    </div>
  )
}
