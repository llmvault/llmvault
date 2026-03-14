import { useQuery } from '@tanstack/react-query'
import { useConnect } from './useConnect'
import type { NangoProvider } from '../types'

const API_URL = import.meta.env.VITE_API_URL || 'https://api.dev.llmvault.dev'

export function useIntegrationProviders() {
  const { sessionId, preview } = useConnect()

  const path = !preview && sessionId
    ? '/v1/widget/integrations/providers'
    : '/v1/integrations/providers'

  const headers: Record<string, string> = {}
  if (!preview && sessionId) {
    headers['Authorization'] = `Bearer ${sessionId}`
  }

  return useQuery<NangoProvider[]>({
    queryKey: ['integration-providers', path],
    queryFn: async () => {
      const res = await fetch(`${API_URL}${path}`, { headers })
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`)
      }
      return res.json()
    },
    staleTime: 5 * 60 * 1000,
  })
}
