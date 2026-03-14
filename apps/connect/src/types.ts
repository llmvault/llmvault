import type { components } from './api/schema'

export type Connection = components['schemas']['connectionResponse']

export interface NangoProvider {
  name: string
  display_name: string
  auth_mode: string
}

export type View =
  | { type: 'provider-selection' }
  | { type: 'api-key-input'; providerId: string }
  | { type: 'validating'; providerId: string }
  | { type: 'success'; providerId: string }
  | { type: 'error'; providerId: string }
  | { type: 'connected-list' }
  | { type: 'provider-detail'; connection: Connection }
  | { type: 'revoke-confirm'; connection: Connection }
  | { type: 'revoke-success'; providerId: string }
  | { type: 'empty-state' }
  | { type: 'integration-selection' }

export type ThemeMode = 'light' | 'dark' | 'system'
