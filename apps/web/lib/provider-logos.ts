export const PROVIDER_LOGOS = {
  ai21: "/logos/ai21.png",
  "aion-labs": "/logos/aion-labs.png",
  alibaba: "/logos/alibaba.png",
  allenai: "/logos/allenai.png",
  amazon: "/logos/amazon.png",
  anthropic: "/logos/anthropic.svg",
  "arcee-ai": "/logos/arcee-ai.png",
  baidu: "/logos/baidu.png",
  "black-forest-labs": "/logos/black-forest-labs.png",
  byteplus: "/logos/byteplus.png",
  bytedance: "/logos/bytedance.png",
  "bytedance-seed": "/logos/bytedance-seed.png",
  cohere: "/logos/cohere.png",
  databricks: "/logos/databricks.png",
  deepcogito: "/logos/deepcogito.png",
  deepseek: "/logos/deepseek.png",
  elevenlabs: "/logos/elevenlabs.svg",
  essentialai: "/logos/essentialai.png",
  featherless: "/logos/featherless.png",
  "fireworks-ai": "/logos/fireworks-ai.svg",
  google: "/logos/google.svg",
  "google-ai-studio": "/logos/google-ai-studio.svg",
  groq: "/logos/groq.svg",
  huggingface: "/logos/huggingface.png",
  "ibm-granite": "/logos/ibm-granite.svg",
  inception: "/logos/inception.png",
  inclusionai: "/logos/inclusionai.png",
  infermatic: "/logos/infermatic.png",
  inflection: "/logos/inflection.png",
  kwaipilot: "/logos/kwaipilot.png",
  kwaivgi: "/logos/kwaivgi.png",
  liquid: "/logos/liquid.png",
  "meta-llama": "/logos/meta-llama.png",
  microsoft: "/logos/microsoft.svg",
  minimax: "/logos/minimax.png",
  mistralai: "/logos/mistralai.png",
  moonshotai: "/logos/moonshotai.png",
  morph: "/logos/morph.jpg",
  "nex-agi": "/logos/nex-agi.svg",
  nousresearch: "/logos/nousresearch.png",
  nvidia: "/logos/nvidia.png",
  openai: "/logos/openai.svg",
  openrouter: "/logos/openrouter.png",
  perceptron: "/logos/perceptron.png",
  perplexity: "/logos/perplexity.svg",
  poolside: "/logos/poolside.svg",
  "prime-intellect": "/logos/prime-intellect.png",
  quiver: "/logos/quiver.svg",
  qwen: "/logos/qwen.png",
  recraft: "/logos/recraft.svg",
  reve: "/logos/reve.svg",
  reflection: "/logos/reflection.png",
  relace: "/logos/relace.png",
  seed: "/logos/seed.png",
  sourceful: "/logos/sourceful.png",
  stepfun: "/logos/stepfun.png",
  "stepfun-ai": "/logos/stepfun-ai.png",
  switchpoint: "/logos/switchpoint.png",
  tencent: "/logos/tencent.png",
  thedrummer: "/logos/thedrummer.png",
  together: "/logos/together.png",
  writer: "/logos/writer.png",
  "x-ai": "/logos/x-ai.png",
  xiaomi: "/logos/xiaomi.png",
  "z-ai": "/logos/z-ai.png",
  zhipuai: "/logos/zhipuai.svg",
} as const

const PROVIDER_LOGO_ALIASES = {
  "byte-plus": "byteplus",
  "deep-seek": "deepseek",
  "hugging-face": "huggingface",
  moonshot: "moonshotai",
  mistral: "mistralai",
  xai: "x-ai",
  zai: "z-ai",
} as const

type KnownProviderLogoID = keyof typeof PROVIDER_LOGOS
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
