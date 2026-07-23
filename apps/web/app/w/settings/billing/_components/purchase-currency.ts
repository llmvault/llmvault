export type PurchaseCurrency = "USD" | "NGN"

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
