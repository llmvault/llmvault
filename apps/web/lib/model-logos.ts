import { providerLogoURL } from "@/lib/provider-logos"

// Keep in sync with canonical model IDs in internal/registry/hivy_models.go.
// Values are OpenRouter model author slugs, resolved to local assets.
const MODEL_LOGO_PROVIDERS = {
  "claude-opus-4.7": "anthropic",
  "claude-opus-4.7-fast": "anthropic",
  "claude-opus-4.6": "anthropic",
  "claude-opus-4.5": "anthropic",
  "claude-sonnet-5": "anthropic",
  "claude-sonnet-4.6": "anthropic",
  "claude-sonnet-4.5": "anthropic",
  "claude-sonnet-4": "anthropic",
  "gpt-5.5": "openai",
  "gpt-5.5-pro": "openai",
  "gpt-5.6-luna": "openai",
  "gpt-5.6-sol": "openai",
  "gpt-5.6-terra": "openai",
  "gpt-5.4": "openai",
  "gpt-5.4-pro": "openai",
  "gpt-5.4-mini": "openai",
  "gpt-5.4-nano": "openai",
  "gpt-4o-mini": "openai",
  "scribe-v2": "elevenlabs",
  "gpt-5.3-codex": "openai",
  "gpt-5.3-codex-spark": "openai",
  "reve-image": "reve",
  "flux.2-klein-4b": "black-forest-labs",
  "riverflow-v2.5-fast": "sourceful",
  "riverflow-v2.5-pro": "sourceful",
  "recraft-v4.1-vector": "recraft",
  "recraft-v4.1-pro-vector": "recraft",
  "arrow-1.1": "quiver",
  "gemini-3.5-flash": "google",
  "gemini-3.1-flash-lite": "google",
  "gemini-3.1-pro-preview": "google",
  "gemini-3-flash-preview": "google",
  "deepseek-v4-pro": "deepseek",
  "deepseek-v4-flash": "deepseek",
  "step-3.7-flash": "stepfun",
  "ling-2.6-1t": "inclusionai",
  "qwen3.7-max": "qwen",
  "qwen3.7-plus": "qwen",
  "grok-4.5": "x-ai",
  "grok-4.3": "x-ai",
  "nemotron-3-ultra-550b-a55b": "nvidia",
  "qwen3.6-max-preview": "qwen",
  "qwen3.6-flash": "qwen",
  "qwen3.6-35b-a3b": "qwen",
  "qwen3.6-27b": "qwen",
  "kimi-k2.7-code": "moonshotai",
  "kimi-k2.6": "moonshotai",
  "mimo-v2.5-pro-ultraspeed": "xiaomi",
  "mimo-v2.5-pro": "xiaomi",
  "mimo-v2.5": "xiaomi",
  "quantised-deepseek-v3.2": "deepseek",
  "quantised-deepseek-v4-pro": "deepseek",
  "quantised-glm-4.7": "z-ai",
  "quantised-glm-4.7-flash": "z-ai",
  "quantised-glm-5.1": "z-ai",
  "quantised-glm-5.2": "z-ai",
  "quantised-kimi-k2.5": "moonshotai",
  "quantised-kimi-k2.5-lightning": "moonshotai",
  "quantised-kimi-k2.6": "moonshotai",
  "quantised-kimi-k2.7-code": "moonshotai",
  "quantised-mimo-v2.5-pro": "xiaomi",
  "quantised-minimax-m2.5": "minimax",
  "quantised-qwen3.5-9b": "qwen",
  "engy-glm-5.2": "z-ai",
  "engy-qwen3.6-35b-a3b": "qwen",
  "minimax-m3": "minimax",
  "minimax-m2.7": "minimax",
  "glm-5.2": "z-ai",
  hy3: "tencent",
  "laguna-m.1": "poolside",
  "glm-5.1": "z-ai",
  "glm-5-turbo": "z-ai",
  "glm-5": "z-ai",
  "glm-4.7": "z-ai",
  "glm-4.7-flash": "z-ai",
  "mistral-small-4": "mistralai",
  "minimax-m2.5": "minimax",
  "step-3.5-flash": "stepfun",
  "kimi-k2.5": "moonshotai",
} as const

type KnownModelID = keyof typeof MODEL_LOGO_PROVIDERS

function logoForProvider(provider: string): string {
  const logo = providerLogoURL(provider)
  if (!logo) {
    throw new Error(`Missing provider logo for ${provider}`)
  }
  return logo
}

export const MODEL_LOGOS = Object.fromEntries(
  Object.entries(MODEL_LOGO_PROVIDERS).map(([modelID, provider]) => [
    modelID,
    logoForProvider(provider),
  ])
) as Record<KnownModelID, string>

export function modelLogoURL(
  modelID: string | null | undefined
): string | undefined {
  if (!modelID) return undefined

  const provider = MODEL_LOGO_PROVIDERS[modelID as KnownModelID]

  return provider ? providerLogoURL(provider) : undefined
}
