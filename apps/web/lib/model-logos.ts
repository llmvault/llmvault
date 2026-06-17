// Keep in sync with canonical model IDs in internal/registry/hivy_models.go.
// Values point to static assets in apps/web/public/logos.
export const MODEL_LOGOS = {
  "claude-opus-4.7": "/logos/anthropic.svg",
  "claude-opus-4.7-fast": "/logos/anthropic.svg",
  "claude-opus-4.6": "/logos/anthropic.svg",
  "claude-opus-4.5": "/logos/anthropic.svg",
  "claude-sonnet-4.6": "/logos/anthropic.svg",
  "claude-sonnet-4.5": "/logos/anthropic.svg",
  "claude-sonnet-4": "/logos/anthropic.svg",
  "gpt-5.5": "/logos/openai.svg",
  "gpt-5.5-pro": "/logos/openai.svg",
  "gpt-5.4": "/logos/openai.svg",
  "gpt-5.4-pro": "/logos/openai.svg",
  "gpt-5.4-mini": "/logos/openai.svg",
  "gpt-5.4-nano": "/logos/openai.svg",
  "gpt-4o-mini": "/logos/openai.svg",
  "gpt-5.3-codex": "/logos/openai.svg",
  "gpt-5.3-codex-spark": "/logos/openai.svg",
  "gemini-3.5-flash": "/logos/google.svg",
  "gemini-3.1-flash-lite": "/logos/google.svg",
  "gemini-3.1-pro-preview": "/logos/google.svg",
  "gemini-3-flash-preview": "/logos/google.svg",
  "deepseek-v4-pro": "/logos/deepseek.svg",
  "deepseek-v4-flash": "/logos/deepseek.svg",
  "step-3.7-flash": "/logos/stepfun.svg",
  "ling-2.6-1t": "/logos/alibaba.svg",
  "qwen3.7-max": "/logos/alibaba.svg",
  "qwen3.7-plus": "/logos/alibaba.svg",
  "grok-4.3": "/logos/xai.svg",
  "nemotron-3-ultra-550b-a55b": "/logos/nvidia.svg",
  "qwen3.6-max-preview": "/logos/alibaba.svg",
  "qwen3.6-flash": "/logos/alibaba.svg",
  "qwen3.6-35b-a3b": "/logos/alibaba.svg",
  "qwen3.6-27b": "/logos/alibaba.svg",
  "kimi-k2.6": "/logos/moonshotai.svg",
  "mimo-v2.5-pro": "/logos/xiaomi.svg",
  "mimo-v2.5": "/logos/xiaomi.svg",
  "minimax-m2.7": "/logos/minimax.svg",
  "glm-5.1": "/logos/zai.svg",
  "glm-5-turbo": "/logos/zai.svg",
  "glm-5": "/logos/zai.svg",
  "glm-4.7": "/logos/zai.svg",
  "glm-4.7-flash": "/logos/zai.svg",
  "mistral-small-4": "/logos/mistral.svg",
  "minimax-m2.5": "/logos/minimax.svg",
  "step-3.5-flash": "/logos/stepfun.svg",
  "kimi-k2.5": "/logos/moonshotai.svg",
} as const

export type KnownModelID = keyof typeof MODEL_LOGOS

export function modelLogoURL(
  modelID: string | null | undefined
): string | undefined {
  if (!modelID) return undefined
  return MODEL_LOGOS[modelID as KnownModelID]
}
