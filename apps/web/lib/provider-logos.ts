export const PROVIDER_LOGOS = {
  anthropic: "/logos/openrouter-anthropic.svg",
  deepseek: "/logos/openrouter-deepseek.png",
  elevenlabs: "/logos/elevenlabs.svg",
  "fireworks-ai": "/logos/fireworks-ai.svg",
  google: "/logos/openrouter-google.svg",
  groq: "/logos/groq.svg",
  inclusionai: "/logos/openrouter-inclusionai.png",
  minimax: "/logos/openrouter-minimax.png",
  mistralai: "/logos/openrouter-mistralai.png",
  moonshotai: "/logos/openrouter-moonshotai.png",
  nvidia: "/logos/openrouter-nvidia.png",
  openai: "/logos/openrouter-openai.svg",
  openrouter: "/logos/openrouter.svg",
  qwen: "/logos/openrouter-qwen.png",
  stepfun: "/logos/openrouter-stepfun.png",
  "x-ai": "/logos/openrouter-x-ai.png",
  xiaomi: "/logos/openrouter-xiaomi.png",
  "z-ai": "/logos/openrouter-z-ai.png",
  zhipuai: "/logos/zhipuai.svg",
} as const

const PROVIDER_LOGO_ALIASES = {
  alibaba: "qwen",
  moonshot: "moonshotai",
  mistral: "mistralai",
  xai: "x-ai",
  zai: "z-ai",
} as const

export type KnownProviderLogoID = keyof typeof PROVIDER_LOGOS
type ProviderLogoAlias = keyof typeof PROVIDER_LOGO_ALIASES

function normalizeProviderSlug(provider: string): string {
  return provider.trim().toLowerCase()
}

export function providerLogoURL(
  provider: string | null | undefined
): string | undefined {
  if (!provider) return undefined

  const normalized = normalizeProviderSlug(provider)
  const aliased =
    PROVIDER_LOGO_ALIASES[normalized as ProviderLogoAlias] ?? normalized

  return PROVIDER_LOGOS[aliased as KnownProviderLogoID]
}
