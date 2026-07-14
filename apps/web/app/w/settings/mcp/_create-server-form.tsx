"use client"

import { useState } from "react"
import { Button, Input, ListBox, Select, Spinner } from "@heroui/react"
import {
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"
import type {
  CreateMcpServerInput,
  McpAuthType,
  McpAuthorizationPolicy,
  McpServerScope,
  McpTransport,
} from "./_lib/mcp-api"
import {
  AUTH_OPTIONS,
  AUTHORIZATION_POLICY_OPTIONS,
  authorizationPrincipal,
  canCreateMcpServer,
  defaultAuthorizationPolicy,
  TRANSPORT_OPTIONS,
} from "./_lib/mcp-ui"
import type { CreateServerFormProps } from "./_create-server-form-shared"

export type { CreateServerSubmission } from "./_create-server-form-shared"

export function CreateServerForm({
  initialScope,
  isAdmin,
  isPending,
  onCancel,
  onCreate,
}: CreateServerFormProps) {
  const scope: McpServerScope =
    initialScope === "org" && isAdmin ? "org" : "personal"
  const [name, setName] = useState("")
  const [url, setURL] = useState("")
  const [transport, setTransport] = useState<McpTransport>("streamable_http")
  const [authType, setAuthType] = useState<McpAuthType>("none")
  const [authorizationPolicy, setAuthorizationPolicy] =
    useState<McpAuthorizationPolicy>("none")
  const [bearerToken, setBearerToken] = useState("")
  const [headerName, setHeaderName] = useState("X-API-Key")
  const [headerValue, setHeaderValue] = useState("")
  const [clientId, setClientId] = useState("")
  const [clientSecret, setClientSecret] = useState("")
  const [tokenUrl, setTokenURL] = useState("")
  const [scopes, setScopes] = useState("")

  const availableAuthOptions = AUTH_OPTIONS.filter(
    (option) => scope === "org" || option.id !== "oauth_client_credentials"
  )
  const selectedAuth = availableAuthOptions.find(
    (option) => option.id === authType
  )
  const canSubmit =
    !isPending &&
    canCreateMcpServer({
      name,
      url,
      scope,
      authType,
      isAdmin,
      bearerToken,
      headerName,
      headerValue,
      clientId,
      clientSecret,
      tokenUrl,
    })

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSubmit) return

    const executionPrincipal = authorizationPrincipal(
      scope,
      authorizationPolicy
    )
    // OAuth application registration belongs to the org configuration even
    // when each member later authorizes their own external identity.
    const authorizationPrincipalType: "user" | "org_service" =
      scope === "org" && authType === "oauth_authorization_code"
        ? "org_service"
        : executionPrincipal
    const authorization =
      authType === "static_bearer"
        ? {
            principal_type: authorizationPrincipalType,
            bearer_token: bearerToken.trim(),
          }
        : authType === "static_header"
          ? {
              principal_type: authorizationPrincipalType,
              header_value: headerValue.trim(),
            }
          : authType === "oauth_authorization_code" && clientId.trim() !== ""
            ? {
                principal_type: authorizationPrincipalType,
                client_id: clientId.trim(),
                client_secret: clientSecret.trim(),
                scopes: scopes
                  .split(/[ ,]+/)
                  .map((value) => value.trim())
                  .filter(Boolean),
              }
            : authType === "oauth_client_credentials"
              ? {
                  principal_type: authorizationPrincipalType,
                  client_id: clientId.trim(),
                  client_secret: clientSecret.trim(),
                  scopes: scopes
                    .split(/[ ,]+/)
                    .map((value) => value.trim())
                    .filter(Boolean),
                }
              : undefined

    const server: CreateMcpServerInput = {
      scope,
      name: name.trim(),
      url: url.trim(),
      transport,
      auth_type: authType,
      authorization_policy: authorizationPolicy,
      ...(authType === "static_header"
        ? { header_name: headerName.trim() }
        : {}),
      ...(authType === "oauth_client_credentials"
        ? { oauth_metadata: { token_endpoint: tokenUrl.trim() } }
        : {}),
      ...(authorization ? { authorization } : {}),
    }

    // Clear plaintext secrets before the network request settles so a failed
    // request cannot leave a credential sitting in the rendered form.
    setBearerToken("")
    setHeaderValue("")
    setClientSecret("")
    await onCreate({
      server,
      startOAuth: authType === "oauth_authorization_code",
      oauthPrincipalType:
        authType === "oauth_authorization_code"
          ? executionPrincipal
          : undefined,
      ...(authType === "oauth_authorization_code"
        ? {
            oauthRegistration: {
              clientId: clientId.trim() || undefined,
              clientSecret: clientSecret.trim() || undefined,
              scopes: scopes
                .split(/[ ,]+/)
                .map((value) => value.trim())
                .filter(Boolean),
            },
          }
        : {}),
    })
  }

  return (
    <form
      onSubmit={submit}
      aria-label="Add MCP server"
      className="flex flex-col gap-6"
    >
      <div>
        <h2 className="text-base font-semibold text-foreground">
          Add {scope === "org" ? "organization" : "personal"} server
        </h2>
        <p className="text-muted-foreground mt-1 text-sm">
          {scope === "org"
            ? "Connect a shared endpoint, then grant it to teams or agents."
            : "Connect an endpoint for agents you use in chats and schedules."}
        </p>
      </div>

      <FormSection
        title="Name"
        description="A short name people will recognize."
      >
        <Input
          aria-label="Name"
          name="name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="Linear tools"
          autoComplete="off"
          className="h-9 w-full rounded-md"
        />
      </FormSection>

      <FormSection
        title="Server URL"
        description="The remote MCP endpoint. Public servers must use HTTPS."
      >
        <Input
          aria-label="Server URL"
          name="url"
          type="url"
          value={url}
          onChange={(event) => setURL(event.target.value)}
          placeholder="https://mcp.example.com/mcp"
          autoComplete="url"
          className="h-9 w-full rounded-md"
        />
      </FormSection>

      <FormSection
        title="Transport"
        description={
          TRANSPORT_OPTIONS.find((option) => option.id === transport)
            ?.description ?? "Choose the protocol this server supports."
        }
      >
        <Select
          aria-label="Transport"
          selectedKey={transport}
          onSelectionChange={(key) => {
            if (key !== null) setTransport(String(key) as McpTransport)
          }}
          className="w-full"
        >
          <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
            <span>
              {
                TRANSPORT_OPTIONS.find((option) => option.id === transport)
                  ?.label
              }
            </span>
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover className="p-1.5">
            <ListBox>
              {TRANSPORT_OPTIONS.map((option) => (
                <ListBox.Item
                  key={option.id}
                  id={option.id}
                  textValue={`${option.label} ${option.description}`}
                >
                  <span className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium">{option.label}</span>
                    <span className="text-muted-foreground text-xs">
                      {option.description}
                    </span>
                  </span>
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
      </FormSection>

      <FormSection
        title="Authentication"
        description={
          selectedAuth?.description ?? "Choose how this server authenticates."
        }
      >
        <Select
          aria-label="Authentication"
          selectedKey={authType}
          onSelectionChange={(key) => {
            if (key !== null) {
              const next = String(key) as McpAuthType
              setAuthType(next)
              setAuthorizationPolicy(defaultAuthorizationPolicy(scope, next))
            }
          }}
          className="w-full"
        >
          <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
            <span>{selectedAuth?.label}</span>
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover className="p-1.5">
            <ListBox>
              {availableAuthOptions.map((option) => (
                <ListBox.Item
                  key={option.id}
                  id={option.id}
                  textValue={`${option.label} ${option.description}`}
                >
                  <span className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium">{option.label}</span>
                    <span className="text-muted-foreground text-xs">
                      {option.description}
                    </span>
                  </span>
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
      </FormSection>

      {scope === "org" &&
      authType !== "none" &&
      authType !== "oauth_client_credentials" ? (
        <FormSection
          title="Authorization ownership"
          description={
            AUTHORIZATION_POLICY_OPTIONS.find(
              (option) => option.id === authorizationPolicy
            )?.description ?? "Choose whose identity agents use."
          }
        >
          <Select
            aria-label="Authorization ownership"
            selectedKey={authorizationPolicy}
            onSelectionChange={(key) => {
              if (key !== null) {
                setAuthorizationPolicy(String(key) as McpAuthorizationPolicy)
              }
            }}
            className="w-full"
          >
            <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
              <span>
                {
                  AUTHORIZATION_POLICY_OPTIONS.find(
                    (option) => option.id === authorizationPolicy
                  )?.label
                }
              </span>
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover className="p-1.5">
              <ListBox>
                {AUTHORIZATION_POLICY_OPTIONS.map((option) => (
                  <ListBox.Item
                    key={option.id}
                    id={option.id}
                    textValue={`${option.label} ${option.description}`}
                  >
                    <span className="flex flex-col gap-0.5">
                      <span className="text-sm font-medium">
                        {option.label}
                      </span>
                      <span className="text-muted-foreground text-xs">
                        {option.description}
                      </span>
                    </span>
                  </ListBox.Item>
                ))}
              </ListBox>
            </Select.Popover>
          </Select>
        </FormSection>
      ) : null}

      {authType === "oauth_authorization_code" ? (
        <>
          <InlineNotice
            icon="shield-check"
            title="Sign in after adding the server"
            body={
              scope === "org" && authorizationPolicy === "service_required"
                ? "This sign-in configures the shared organization identity."
                : scope === "org"
                  ? "Every member connects their own account."
                  : "The authorization belongs only to you."
            }
          />
          <FormSection
            title="OAuth client ID (optional)"
            description="Hivy registers a client automatically when the server supports it."
          >
            <Input
              aria-label="OAuth client ID"
              name="client-id"
              value={clientId}
              onChange={(event) => setClientId(event.target.value)}
              autoComplete="username"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
          <FormSection
            title="OAuth client secret (optional)"
            description="Leave this empty for public PKCE clients."
          >
            <Input
              aria-label="OAuth client secret"
              name="client-secret"
              type="password"
              value={clientSecret}
              onChange={(event) => setClientSecret(event.target.value)}
              autoComplete="new-password"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
          <FormSection
            title="Scopes (optional)"
            description="Separate scopes with spaces or commas."
          >
            <Input
              aria-label="Scopes"
              name="scopes"
              value={scopes}
              onChange={(event) => setScopes(event.target.value)}
              placeholder="read write"
              autoComplete="off"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
        </>
      ) : null}

      {authType === "static_bearer" ? (
        <FormSection
          title="Bearer token"
          description="Stored encrypted and never shown again."
        >
          <Input
            aria-label="Bearer token"
            name="bearer-token"
            type="password"
            value={bearerToken}
            onChange={(event) => setBearerToken(event.target.value)}
            placeholder="Paste token"
            autoComplete="new-password"
            className="h-9 w-full rounded-md"
          />
        </FormSection>
      ) : null}

      {authType === "static_header" ? (
        <>
          <FormSection
            title="Header name"
            description="For example, X-API-Key."
          >
            <Input
              aria-label="Header name"
              name="header-name"
              value={headerName}
              onChange={(event) => setHeaderName(event.target.value)}
              autoComplete="off"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
          <FormSection
            title="Header value"
            description="Stored encrypted and never shown again."
          >
            <Input
              aria-label="Header value"
              name="header-value"
              type="password"
              value={headerValue}
              onChange={(event) => setHeaderValue(event.target.value)}
              placeholder="Paste API key"
              autoComplete="new-password"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
        </>
      ) : null}

      {authType === "oauth_client_credentials" ? (
        <>
          <FormSection
            title="Client ID"
            description="The OAuth application identifier."
          >
            <Input
              aria-label="Client ID"
              name="client-id"
              value={clientId}
              onChange={(event) => setClientId(event.target.value)}
              autoComplete="username"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
          <FormSection
            title="Client secret"
            description="Stored encrypted and never shown again."
          >
            <Input
              aria-label="Client secret"
              name="client-secret"
              type="password"
              value={clientSecret}
              onChange={(event) => setClientSecret(event.target.value)}
              autoComplete="new-password"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
          <FormSection
            title="Token URL"
            description="The OAuth token endpoint for this server."
          >
            <Input
              aria-label="Token URL"
              name="token-url"
              type="url"
              value={tokenUrl}
              onChange={(event) => setTokenURL(event.target.value)}
              placeholder="https://auth.example.com/oauth/token"
              autoComplete="url"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
          <FormSection
            title="Scopes (optional)"
            description="Separate scopes with spaces or commas."
          >
            <Input
              aria-label="Scopes"
              name="scopes"
              value={scopes}
              onChange={(event) => setScopes(event.target.value)}
              placeholder="read write"
              autoComplete="off"
              className="h-9 w-full rounded-md"
            />
          </FormSection>
        </>
      ) : null}

      <div className="flex items-center justify-end gap-2">
        <Button
          type="button"
          variant="tertiary"
          size="sm"
          isDisabled={isPending}
          onPress={onCancel}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          variant="primary"
          size="sm"
          isDisabled={!canSubmit}
        >
          {isPending ? <Spinner color="current" size="sm" /> : null}
          Add server
        </Button>
      </div>
    </form>
  )
}
