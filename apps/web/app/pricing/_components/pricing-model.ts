export const CREDIT_USD_VALUE = 0.001
export const DEPOSIT_FEE_RATE = 0.12

export type DepositEstimate = {
  creditValue: number
  creditsAdded: number
  depositFee: number
  checkoutTotal: number
}

function nonNegative(value: number) {
  return Number.isFinite(value) ? Math.max(0, value) : 0
}

export function calculateDeposit(value: number): DepositEstimate {
  const creditValue = nonNegative(value)
  const depositFee = creditValue * DEPOSIT_FEE_RATE

  return {
    creditValue,
    creditsAdded: Math.round(creditValue / CREDIT_USD_VALUE),
    depositFee,
    checkoutTotal: creditValue + depositFee,
  }
}
