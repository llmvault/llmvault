export type PurchaseCurrency = "USD" | "NGN"

type CustomPurchaseQuote = {
  subtotalMinor: number
  feeMinor: number
  totalMinor: number
  credits: number
}

export function filterByPurchaseCurrency<T extends { currency?: string }>(
  items: T[],
  currency: PurchaseCurrency
): T[] {
  return items.filter((item) => item.currency === currency)
}

export function resolveCompatiblePaymentMethodID(
  methods: Array<{ id?: string }>,
  selectedID: string | null
): string {
  if (selectedID === "new") return "new"
  if (selectedID && methods.some((method) => method.id === selectedID)) {
    return selectedID
  }
  return methods.find((method) => method.id)?.id ?? "new"
}

export function quoteCustomPurchase(
  amount: string,
  currency: PurchaseCurrency,
  feeBasisPoints: number,
  ngnMinorPerUSD: number
): CustomPurchaseQuote | null {
  const match = amount.trim().match(/^(\d*)(?:\.(\d{0,2}))?$/)
  if (!match || (!match[1] && !match[2])) return null
  if (!Number.isInteger(feeBasisPoints) || feeBasisPoints < 0) return null

  const major = BigInt(match[1] || "0")
  const fraction = BigInt((match[2] || "").padEnd(2, "0"))
  const subtotal = major * BigInt(100) + fraction
  if (subtotal <= BigInt(0)) return null

  let credits: bigint
  if (currency === "USD") {
    credits = subtotal * BigInt(10)
  } else {
    if (!Number.isSafeInteger(ngnMinorPerUSD) || ngnMinorPerUSD <= 0) {
      return null
    }
    credits = (subtotal * BigInt(1000)) / BigInt(ngnMinorPerUSD)
  }
  if (credits <= BigInt(0)) return null

  const fee =
    (subtotal * BigInt(feeBasisPoints) + BigInt(9_999)) / BigInt(10_000)
  const total = subtotal + fee
  const maxSafeInteger = BigInt(Number.MAX_SAFE_INTEGER)
  if (
    subtotal > maxSafeInteger ||
    fee > maxSafeInteger ||
    total > maxSafeInteger ||
    credits > maxSafeInteger
  ) {
    return null
  }

  return {
    subtotalMinor: Number(subtotal),
    feeMinor: Number(fee),
    totalMinor: Number(total),
    credits: Number(credits),
  }
}
