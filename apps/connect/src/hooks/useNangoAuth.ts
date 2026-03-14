import { useMutation } from '@tanstack/react-query'
import Nango from '@nangohq/frontend'
import { useConnect } from './useConnect'
import { createWidgetFetchClient } from '../api/client'

const INTEGRATIONS_API = import.meta.env.VITE_INTEGRATIONS_API || 'https://integrations.dev.llmvault.dev'

export function useNangoAuth(integrationId: string) {
  const { sessionId } = useConnect()

  return useMutation({
    mutationFn: async () => {
      const client = createWidgetFetchClient(sessionId!)

      const { data: sessionData } = await client.POST(
        '/v1/widget/integrations/{id}/connect-session',
        { params: { path: { id: integrationId } } }
      )

      const token = sessionData?.token
      const providerConfigKey = sessionData?.provider_config_key
      if (!token || !providerConfigKey) {
        throw new Error('Failed to create connect session')
      }

      const nango = new Nango({ connectSessionToken: token, host: INTEGRATIONS_API })

      const result = await nango.auth(providerConfigKey, { detectClosedAuthWindow: true })

      await client.POST('/v1/widget/integrations/{id}/connections', {
        params: { path: { id: integrationId } },
        body: { nango_connection_id: result.connectionId },
      })
    },
  })
}
