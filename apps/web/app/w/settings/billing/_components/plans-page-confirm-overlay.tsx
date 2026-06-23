import { Button, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
import { DetailRow } from "./plans-modal-components"

export function PlanChangeConfirmOverlay({
  actionLabel,
  anyPending,
  applyPending,
  confirmIsUpgrade,
  initPending,
  rows,
  onCancel,
  onConfirm,
}: {
  actionLabel: string
  anyPending: boolean
  applyPending: boolean
  confirmIsUpgrade: boolean
  initPending: boolean
  rows: { label: string; value: string; emphasis?: boolean }[]
  onCancel: () => void
  onConfirm: () => void
}) {
  const isProcessing = applyPending || initPending

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="billing-plan-change-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4 backdrop-blur-sm"
    >
      <div className="bg-overlay shadow-overlay relative flex w-full max-w-md flex-col rounded-3xl p-8 outline-none">
        <button
          type="button"
          aria-label="Close"
          onClick={onCancel}
          disabled={isProcessing}
          className="hover:bg-default absolute top-4 right-4 flex h-8 w-8 items-center justify-center rounded-lg text-muted transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
        >
          <Icon icon="lucide:x" className="h-4 w-4" />
        </button>

        <div className="flex flex-col gap-6">
          <div className="flex flex-col gap-4">
            <div className="bg-default flex size-12 items-center justify-center rounded-2xl text-foreground">
              <Icon icon="lucide:credit-card" className="h-6 w-6" />
            </div>
            <div className="flex flex-col gap-1 pr-8">
              <h2
                id="billing-plan-change-title"
                className="text-lg font-semibold text-foreground"
              >
                {confirmIsUpgrade ? "Confirm upgrade" : "Confirm downgrade"}
              </h2>
              <p className="text-sm text-muted">
                {confirmIsUpgrade
                  ? "Review the prorated charge before continuing."
                  : "This change is scheduled for the end of your current billing period."}
              </p>
            </div>
          </div>

          <div className="bg-surface overflow-hidden rounded-2xl border border-border">
            {rows.map((row, index) => (
              <DetailRow
                key={row.label}
                label={row.label}
                value={row.value}
                emphasis={row.emphasis}
                last={index === rows.length - 1}
              />
            ))}
          </div>

          <div className="flex items-center justify-end gap-2">
            <Button
              variant="tertiary"
              size="sm"
              isDisabled={isProcessing}
              onPress={onCancel}
            >
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              isDisabled={anyPending}
              onPress={onConfirm}
            >
              {isProcessing ? <Spinner color="current" size="sm" /> : null}
              {actionLabel}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
