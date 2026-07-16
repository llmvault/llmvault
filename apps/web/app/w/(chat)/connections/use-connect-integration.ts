"use client"

import posthog from "posthog-js"
import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { extractErrorMessage } from "@/lib/api/error"
import { clientConfig } from "@/lib/config/public-config"
import type { components } from "@/lib/api/schema"

type Connection = components["schemas"]["connectionResponse"]

export interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

interface ConnectIntegrationOptions extends ConnectOptions {
  onSuccess?: (connection: Connection) => void
}

interface ReconnectIntegrationOptions extends ConnectOptions {
  onSuccess?: () => void
}

export function useConnectIntegration() {
  const queryClient = useQueryClient()
  const createConnectSession = $api.useMutation(
    "post",
    "/v1/integrations/{id}/connect-session"
  )
  const createReconnectSession = $api.useMutation(
    "post",
    "/v1/connections/{id}/reconnect-session"
  )
  const createConnection = $api.useMutation(
    "post",
    "/v1/integrations/{id}/connections"
  )
  const [connectingId, setConnectingId] = useState<string | null>(null)

  async function runConnect(integrationId: string, options?: ConnectOptions) {
    const session = await createConnectSession.mutateAsync({
      params: { path: { id: integrationId } },
    })

    const token = session?.token
    const providerConfigKey = session?.provider_config_key
    if (!token || !providerConfigKey) {
      throw new Error("Could not start connection")
    }

    const nango = new Nango({
      connectSessionToken: token,
      host: clientConfig().connectionsHost,
    })

    const authOptions: Record<string, unknown> = {}
    if (options?.credentials) authOptions.credentials = options.credentials
    if (options?.params) authOptions.params = options.params
    if (options?.installation) authOptions.installation = options.installation

    const authResult =
      Object.keys(authOptions).length > 0
        ? await nango.auth(providerConfigKey, authOptions)
        : await nango.auth(providerConfigKey)

    const connection = await createConnection.mutateAsync({
      params: { path: { id: integrationId } },
      body: {
        nango_connection_id: authResult.connectionId,
        meta:
          options?.params && Object.keys(options.params).length > 0
            ? { connection_config: options.params }
            : undefined,
      },
    })

    queryClient.invalidateQueries({ queryKey: queryKeys.connections() })
    queryClient.invalidateQueries({ queryKey: queryKeys.connections() })
    return connection
  }

  async function runReconnect(connectionId: string, options?: ConnectOptions) {
    const session = await createReconnectSession.mutateAsync({
      params: { path: { id: connectionId } },
    })

    const token = session?.token
    const providerConfigKey = session?.provider_config_key
    if (!token || !providerConfigKey) {
      throw new Error("Could not start reconnect")
    }

    const nango = new Nango({
      connectSessionToken: token,
      host: clientConfig().connectionsHost,
    })

    const authOptions: Record<string, unknown> = {}
    if (options?.credentials) authOptions.credentials = options.credentials
    if (options?.params) authOptions.params = options.params
    if (options?.installation) authOptions.installation = options.installation

    if (Object.keys(authOptions).length > 0) {
      await nango.reconnect(providerConfigKey, authOptions)
    } else {
      await nango.reconnect(providerConfigKey)
    }

    queryClient.invalidateQueries({ queryKey: queryKeys.connections() })
    queryClient.invalidateQueries({ queryKey: queryKeys.connections() })
  }

  function connectIntegration(
    integrationId: string,
    options?: ConnectIntegrationOptions
  ) {
    const { onSuccess, ...connectOptions } = options ?? {}
    const normalizedOptions =
      Object.keys(connectOptions).length > 0 ? connectOptions : undefined

    setConnectingId(integrationId)
    runConnect(integrationId, normalizedOptions)
      .then((connection) => {
        posthog.capture("connection_created", {
          integration_id: integrationId,
        })
        if (connection) onSuccess?.(connection)
      })
      .catch((error) => {
        if (error instanceof AuthError && error.type === "window_closed") return
        toast.danger(extractErrorMessage(error, "Connection failed"))
      })
      .finally(() => {
        setConnectingId(null)
      })
  }

  function reconnectIntegration(
    connectionId: string,
    options?: ReconnectIntegrationOptions
  ) {
    const { onSuccess, ...connectOptions } = options ?? {}
    const normalizedOptions =
      Object.keys(connectOptions).length > 0 ? connectOptions : undefined

    setConnectingId(connectionId)
    runReconnect(connectionId, normalizedOptions)
      .then(() => {
        onSuccess?.()
      })
      .catch((error) => {
        if (error instanceof AuthError && error.type === "window_closed") return
        toast.danger(extractErrorMessage(error, "Reconnect failed"))
      })
      .finally(() => {
        setConnectingId(null)
      })
  }

  return {
    connectIntegration,
    reconnectIntegration,
    connectingId,
    isConnecting:
      createConnectSession.isPending ||
      createReconnectSession.isPending ||
      createConnection.isPending ||
      connectingId !== null,
  }
}
