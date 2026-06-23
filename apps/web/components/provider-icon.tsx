import { ProviderLogo } from "@/components/provider-logo"

export function ProviderIcon({ provider }: { provider: string }) {
  return <ProviderLogo provider={provider} size={20} />
}
