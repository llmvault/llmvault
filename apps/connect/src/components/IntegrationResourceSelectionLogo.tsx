import { IntegrationProviderLogo } from './IntegrationProviderLogo'

interface Props {
  providerName: string
  size?: string
  rounded?: string
}

export function IntegrationResourceSelectionLogo({ providerName, size = 'size-10', rounded = 'rounded-lg' }: Props) {
  return <IntegrationProviderLogo providerName={providerName} size={size} rounded={rounded} />
}
