import { $api, createWidgetApi } from '../api/client'
import { useConnect } from './useConnect'

export function useIntegrationProviders() {
  const { sessionId, preview } = useConnect()

  if (!preview && sessionId) {
    const widgetApi = createWidgetApi(sessionId)
    return widgetApi.useQuery('get', '/v1/widget/integrations/providers', undefined, {
      staleTime: 5 * 60 * 1000,
    })
  }

  return $api.useQuery('get', '/v1/integrations/providers', undefined, {
    staleTime: 5 * 60 * 1000,
  })
}
