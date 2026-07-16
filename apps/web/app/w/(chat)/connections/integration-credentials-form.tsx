"use client"

import { useMemo, useState } from "react"
import { Button, Input, Label, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import {
  type AvailableIntegration,
  integrationConnectionFields,
} from "@/app/w/(chat)/connections/integration-auth"
import type { ConnectOptions } from "@/app/w/(chat)/connections/use-connect-integration"

export function IntegrationCredentialsForm({
  integration,
  isSubmitting,
  onBack,
  onSubmit,
}: {
  integration: AvailableIntegration
  isSubmitting: boolean
  onBack: () => void
  onSubmit: (options?: ConnectOptions) => void
}) {
  const config = integration.nango_config
  const authMode = config?.auth_mode
  const installation = config?.installation
  const isApiKey = authMode === "API_KEY"
  const isBasic = authMode === "BASIC"
  const isOutbound = installation === "outbound"
  const fields = useMemo(
    () => integrationConnectionFields(integration),
    [integration]
  )

  const [apiKey, setApiKey] = useState("")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [configValues, setConfigValues] = useState<Record<string, string>>({})

  function setConfigValue(key: string, value: string) {
    setConfigValues((current) => ({ ...current, [key]: value }))
  }

  function isValid() {
    if (isApiKey && !apiKey.trim()) return false
    if (isBasic && (!username.trim() || !password.trim())) return false

    for (const field of fields) {
      const value = configValues[field.key] ?? ""
      if (!value.trim()) return false
      if (field.pattern) {
        try {
          if (!new RegExp(field.pattern).test(value)) return false
        } catch {
          // Ignore invalid provider-supplied patterns.
        }
      }
    }

    return true
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!isValid() || isSubmitting) return

    const options: ConnectOptions = {}
    if (isApiKey) {
      options.credentials = { apiKey: apiKey.trim() }
    } else if (isBasic) {
      options.credentials = {
        username: username.trim(),
        password: password.trim(),
      }
    } else if (isOutbound) {
      options.credentials = {}
    }

    const params: Record<string, string> = {}
    for (const field of fields) {
      const value = configValues[field.key]?.trim()
      if (value) params[field.key] = value
    }
    if (Object.keys(params).length > 0) options.params = params
    if (isOutbound) options.installation = "outbound"

    onSubmit(Object.keys(options).length > 0 ? options : undefined)
  }

  const description = isApiKey
    ? `Enter your API key to connect ${integration.display_name}.`
    : isOutbound
      ? `Enter the required details, then complete the installation from ${integration.display_name}'s settings.`
      : `Enter the required details to connect ${integration.display_name}.`

  return (
    <div className="flex max-h-[82vh] flex-col overflow-hidden p-6">
      <div className="mb-6 flex items-center gap-3">
        {!isApiKey ? (
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            onPress={onBack}
            aria-label="Back"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
          </Button>
        ) : null}
        <IntegrationLogo provider={integration.provider ?? ""} size={28} />
        <div className="min-w-0">
          <h2 className="text-lg font-semibold text-foreground">
            Connect {integration.display_name}
          </h2>
          <p className="text-sm text-muted">{description}</p>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        {isApiKey ? (
          <label className="flex flex-col gap-2">
            <Label htmlFor="connection-api-key">API key</Label>
            <Input
              id="connection-api-key"
              type="password"
              value={apiKey}
              onChange={(event) => setApiKey(event.target.value)}
              placeholder="Enter your API key"
              autoFocus
            />
          </label>
        ) : null}

        {isBasic ? (
          <>
            <label className="flex flex-col gap-2">
              <Label htmlFor="connection-username">Username</Label>
              <Input
                id="connection-username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder="Enter username"
                autoFocus
              />
            </label>
            <label className="flex flex-col gap-2">
              <Label htmlFor="connection-password">Password</Label>
              <Input
                id="connection-password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="Enter password"
              />
            </label>
          </>
        ) : null}

        {fields.map((field, index) => (
          <label key={field.key} className="flex flex-col gap-2">
            <Label htmlFor={`connection-${field.key}`}>{field.title}</Label>
            <Input
              id={`connection-${field.key}`}
              value={configValues[field.key] ?? ""}
              onChange={(event) =>
                setConfigValue(field.key, event.target.value)
              }
              placeholder={field.placeholder}
              autoFocus={!isApiKey && !isBasic && index === 0}
            />
            {field.description ? (
              <span className="text-xs leading-5 text-muted">
                {field.description}
              </span>
            ) : null}
          </label>
        ))}

        {isOutbound ? (
          <p className="bg-default rounded-2xl px-3 py-2 text-xs leading-5 text-muted">
            After submitting, go to {integration.display_name}&apos;s settings
            to accept and install the integration.
          </p>
        ) : null}

        <Button
          type="submit"
          variant="primary"
          fullWidth
          className="mt-2 rounded-full"
          isDisabled={!isValid() || isSubmitting}
        >
          {isSubmitting ? <Spinner color="current" size="sm" /> : null}
          Connect
        </Button>
      </form>
    </div>
  )
}
