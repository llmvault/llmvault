const CREDIT_USD_VALUE = 0.001

export function creditsToUsageUSD(credits: number | undefined | null) {
  return (credits ?? 0) * CREDIT_USD_VALUE
}

export function formatCreditLabel(credits: number) {
  if (credits >= 1000) {
    const thousands = credits / 1000
    return `${Number.isInteger(thousands) ? thousands : thousands.toFixed(1)}K`
  }
  return credits.toLocaleString("en-NG")
}

export function formatUsageValue(credits: number | undefined | null) {
  const value = creditsToUsageUSD(credits)
  const isWholeDollar = Number.isInteger(value)
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: isWholeDollar ? 0 : 2,
    maximumFractionDigits: 2,
  }).format(value)
}

export function formatCreditsWithUsage(credits: number | undefined | null) {
  const count = (credits ?? 0).toLocaleString("en-NG")
  return `${count} credits · ${formatUsageValue(credits)} usage value`
}
