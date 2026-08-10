const CREDIT_USD_VALUE = 0.001

export interface SessionUsageSnapshot {
  cost_usd?: number
  credits?: number
  model_cost_usd?: number
  model_credits?: number
  sandbox_cost_usd?: number
  sandbox_credits?: number
  sandbox_vcpu_seconds?: number
}

export interface SessionUsageSummary {
  costUsd: number
  credits: number
  modelCostUsd: number
  modelCredits: number
  sandboxCostUsd: number
  sandboxCredits: number
  sandboxVCPUSeconds: number
  updatedAt: number
}

export function sessionUsageSummaryFromSnapshot(
  snapshot: SessionUsageSnapshot
): SessionUsageSummary {
  const costUsd = positiveNumber(snapshot.cost_usd)
  const credits = positiveNumber(snapshot.credits) || creditsForCostUSD(costUsd)
  const sandboxCredits = nonNegativeNumber(snapshot.sandbox_credits)
  const sandboxCostUsd =
    nonNegativeNumber(snapshot.sandbox_cost_usd) ||
    sandboxCredits * CREDIT_USD_VALUE
  const modelCredits =
    nonNegativeNumber(snapshot.model_credits) ||
    Math.max(0, credits - sandboxCredits)
  const modelCostUsd =
    nonNegativeNumber(snapshot.model_cost_usd) ||
    Math.max(0, costUsd - sandboxCostUsd)
  return {
    costUsd,
    credits,
    modelCostUsd,
    modelCredits,
    sandboxCostUsd,
    sandboxCredits,
    sandboxVCPUSeconds: nonNegativeNumber(snapshot.sandbox_vcpu_seconds),
    updatedAt: Date.now(),
  }
}

export function creditsForCostUSD(costUsd: number) {
  if (!Number.isFinite(costUsd) || costUsd <= 0) return 0
  return Math.ceil(costUsd / CREDIT_USD_VALUE)
}

export function formatSessionCostUSD(costUsd: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(positiveNumber(costUsd))
}

export function formatSessionCredits(credits: number) {
  return new Intl.NumberFormat("en-US", {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(nonNegativeNumber(credits))
}

export function modelUsageEventKey(payload: Record<string, unknown>) {
  const usage = payloadRecord(payload.usage)
  return JSON.stringify([
    stringValue(payload.turn_id),
    stringValue(payload.model),
    numberValue(usage.prompt_tokens),
    numberValue(usage.completion_tokens),
    numberValue(usage.total_tokens),
    numberValue(usage.cached_tokens),
    numberValue(usage.cache_write_tokens),
    numberValue(usage.reasoning_tokens),
    numberValue(usage.cost),
  ])
}

export function modelUsageCost(payload: Record<string, unknown>) {
  const cost = numberValue(payloadRecord(payload.usage).cost)
  return cost > 0 ? cost : 0
}

function payloadRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value : ""
}

function numberValue(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0
}

function positiveNumber(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) && value > 0
    ? value
    : 0
}

function nonNegativeNumber(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : 0
}
